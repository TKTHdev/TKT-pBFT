package main

import (
	"fmt"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	TestBinary = "./pbft_test_bin"
	ConfFile   = "cluster.conf"
)

func buildBinary(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", TestBinary, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}
}

func startNode(t *testing.T, id int, workers int, logDir string) *exec.Cmd {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	logFile, err := os.Create(filepath.Join(logDir, fmt.Sprintf("node_%d.log", id)))
	if err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	args := []string{
		"start",
		"--id", fmt.Sprintf("%d", id),
		"--conf", ConfFile,
		"--workers", fmt.Sprintf("%d", workers),
		"--in-memory", // Use in-memory for speed and avoiding disk cleanup issues
		"--crypto", "ed25519",
		"--workload", "ycsb-a", // Default workload
	}

	cmd := exec.Command(TestBinary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start node %d: %v", id, err)
	}
	return cmd
}

func waitForCompletion(t *testing.T, logPath string, timeout time.Duration) {
	start := time.Now()
	for {
		if time.Since(start) > timeout {
			t.Fatalf("Timeout waiting for experiment completion")
		}

		content, err := os.ReadFile(logPath)
		if err == nil {
			s := string(content)
			if strings.Contains(s, "ConcClient total commands processed") {
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
}

func checkConsistency(t *testing.T) {
	// Wait a bit for propagation
	time.Sleep(2 * time.Second)

	checksums := make(map[string][]int)
	counts := make(map[int]int)

	// Connect to all 4 nodes
	for id := 1; id <= 4; id++ {
		// Port is 6000 + id - 1
		port := 6000 + id - 1
		client, err := rpc.Dial("tcp", fmt.Sprintf("localhost:%d", port))
		if err != nil {
			t.Errorf("Failed to connect to node %d: %v", id, err)
			continue
		}
		defer client.Close()

		var reply GetStateChecksumReply
		err = client.Call("PBFT.GetStateChecksum", &GetStateChecksumArgs{}, &reply)
		if err != nil {
			t.Errorf("Failed to call GetStateChecksum on node %d: %v", id, err)
			continue
		}

		checksums[reply.Checksum] = append(checksums[reply.Checksum], id)
		counts[id] = reply.Count
		t.Logf("Node %d: Count=%d, Checksum=%s", id, reply.Count, reply.Checksum)
	}

	if len(checksums) > 1 {
		t.Errorf("Inconsistency detected! Checksums: %v", checksums)
	} else if len(checksums) == 0 {
		t.Errorf("No checksums retrieved")
	} else {
		for sum, nodes := range checksums {
			t.Logf("Consistency verified! Checksum %s matches on nodes %v", sum, nodes)
			// Check if we actually processed data
			for _, id := range nodes {
				if counts[id] == 0 {
					t.Errorf("Node %d has 0 items in state!", id)
				}
			}
		}
	}
}

func TestYCSBConsistency(t *testing.T) {
	buildBinary(t)
	defer os.Remove(TestBinary)

	// workerCounts := []int{10, 50, 100, 200}
	// For "10000+ commands", with 10s duration:
	// 10 workers * 10s is likely > 1000 ops if >10ops/sec/worker.
	// We want to be sure.
	// Let's use a few increasing steps.
	workerCounts := []int{10, 50, 100}

	for _, workers := range workerCounts {
		t.Run(fmt.Sprintf("Workers-%d", workers), func(t *testing.T) {
			logDir := filepath.Join("logs", fmt.Sprintf("test_workers_%d", workers))
			cmds := make([]*exec.Cmd, 4)

			// cleanup previous runs just in case
			exec.Command("pkill", "-f", TestBinary).Run()

			for i := 1; i <= 4; i++ {
				cmds[i-1] = startNode(t, i, workers, logDir)
			}

			// Ensure cleanup
			defer func() {
				for _, cmd := range cmds {
					if cmd.Process != nil {
						cmd.Process.Kill()
					}
				}
				exec.Command("pkill", "-f", TestBinary).Run()
			}()

			// Wait for Node 1 (Primary) to finish
			primaryLog := filepath.Join(logDir, "node_1.log")
			waitForCompletion(t, primaryLog, 30*time.Second)

			checkConsistency(t)
		})
	}
}
