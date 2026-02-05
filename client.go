package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/sourcegraph/conc/pool"
)

const (
	VALUE_MAX           = 1500
	CLIENT_START        = 4000 * time.Millisecond
	EXPERIMENT_DURATION = 10000 * time.Millisecond
	YCSB_A              = 50
	YCSB_B              = 5
	YCSB_C              = 0
)

type Response struct {
	success bool
	value   string
}

type WorkerResult struct {
	count    int
	duration time.Duration
}

type Client struct {
	sendCh        chan []byte
	internalState map[string]string
}

func (c *Client) randomKey() string {
	keys := []string{"x", "y", "z", "a", "b", "c"}
	return keys[rand.Intn(len(keys))]
}

func (c *Client) randomValue() string {
	return fmt.Sprintf("value%d", rand.Intn(VALUE_MAX))
}

func (c *Client) createYCSBCommand(writeRatio int) []byte {
	opRand := rand.Intn(100)
	if opRand < writeRatio {
		key := c.randomKey()
		value := c.randomValue()
		commandString := fmt.Sprintf("SET %s %s", key, value)
		return []byte(commandString)
	} else {
		key := c.randomKey()
		commandString := fmt.Sprintf("GET %s", key)
		return []byte(commandString)
	}
}

func (p *PBFT) concClient() {
	// Wait until we are ready or just start after a delay
	time.Sleep(CLIENT_START)

	if !p.isPrimary() {
		// Only primary generates load in this setup
		fmt.Println("Not primary, not starting client.")
		return
	}

	fmt.Println("ConcClient starting experiment (Primary)...")

	ctx, cancel := context.WithTimeout(context.Background(), EXPERIMENT_DURATION)
	defer cancel()

	pool := pool.NewWithResults[WorkerResult]().WithErrors().WithMaxGoroutines(p.workers)
	for i := 0; i < p.workers; i++ {
		pool.Go(func() (WorkerResult, error) { return concClientWorker(ctx, p) })
	}
	results, err := pool.Wait()

	if err != nil {
		fmt.Println("ConcClient encountered error:", err)
		return
	}
	totalCommands := 0
	var totalDuration time.Duration
	for _, res := range results {
		totalCommands += res.count
		totalDuration += res.duration
	}
	throughput := float64(totalCommands) / EXPERIMENT_DURATION.Seconds()
	avgLatency := float64(0)
	if totalCommands > 0 {
		avgLatency = float64(totalDuration.Milliseconds()) / float64(totalCommands)
	}

	fmt.Printf("ConcClient total commands processed: %d\n", totalCommands)
	fmt.Printf("ConcClient throughput: %.2f commands/second\n", throughput)
	fmt.Printf("ConcClient average latency: %.2f ms\n", avgLatency)

	// CSV output for makefile to capture
	workloadName := "unknown"
	switch p.workload {
	case 50:
		workloadName = "ycsb-a"
	case 5:
		workloadName = "ycsb-b"
	case 0:
		workloadName = "ycsb-c"
	}
	fmt.Printf("RESULT:%s,%d,%d,%d,%.2f,%.2f\n", workloadName, p.readBatchSize, p.writeBatchSize, p.workers, throughput, avgLatency)
}

func concClientWorker(ctx context.Context, p *PBFT) (WorkerResult, error) {
	client := &Client{
		internalState: make(map[string]string),
	}
	res := WorkerResult{}
	for {
		select {
		case <-ctx.Done():
			return res, nil
		default:
		}

		// Always submit if we are running this worker (checked isPrimary before starting)
		command := client.createYCSBCommand(p.workload)
		req := ClientRequest{
			Command: command,
			RespCh:  make(chan Response, 1),
		}

		start := time.Now()
		select {
		case <-ctx.Done():
			return res, nil
		case p.ReqCh <- req:
		}

		select {
		case <-ctx.Done():
			return res, nil
		case resp := <-req.RespCh:
			if resp.success {
				res.count += 1
				res.duration += time.Since(start)
			} else {
				// fmt.Println("ConcClient command failed:", string(command))
				return res, fmt.Errorf("command failed")
			}
		}
	}
}
