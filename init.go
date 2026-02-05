package main

import (
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "pbft",
		Usage: "A simple PBFT implementation",
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start the PBFT node",
				Action: func(c *cli.Context) error {
					id := c.Int("id")
					conf := c.String("conf")
					writeBatchSize := c.Int("write-batch-size")
					readBatchSize := c.Int("read-batch-size")
					workers := c.Int("workers")
					debug := c.Bool("debug")
					asyncLog := c.Bool("async-log")
					inMemory := c.Bool("in-memory")
					workloadStr := c.String("workload")
					cryptoStr := c.String("crypto")
					workload := 50
					switch workloadStr {
					case "ycsb-a":
						workload = 50
					case "ycsb-b":
						workload = 5
					case "ycsb-c":
						workload = 0
					}
					cryptoType := CryptoEd25519
					switch cryptoStr {
					case "ed25519":
						cryptoType = CryptoEd25519
					case "mac":
						cryptoType = CryptoMAC
					}
					p := NewPBFT(id, conf, writeBatchSize, readBatchSize, workers, debug, workload, asyncLog, inMemory, cryptoType)
					p.Run()
					return nil
				},
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  "id",
						Usage: "Node ID",
					},
					&cli.StringFlag{
						Name:  "conf",
						Usage: "Path to config file",
					},
					&cli.IntFlag{
						Name:  "write-batch-size",
						Usage: "PBFT disk write batch size",
						Value: 128,
					},
					&cli.IntFlag{
						Name:  "read-batch-size",
						Usage: "PBFT read batch size",
						Value: 128,
					},
					&cli.IntFlag{
						Name:  "workers",
						Usage: "Number of concurrent clients",
						Value: 256,
					},
					&cli.BoolFlag{
						Name:  "debug",
						Usage: "Enable debug logging",
						Value: false,
					},
					&cli.BoolFlag{
						Name:  "async-log",
						Usage: "Enable asynchronous disk writes",
						Value: false,
					},
					&cli.BoolFlag{
						Name:  "in-memory",
						Usage: "Use /dev/shm for storage (Linux only)",
						Value: false,
					},
					&cli.StringFlag{
						Name:  "workload",
						Usage: "Workload type (ycsb-a, ycsb-b, ycsb-c)",
						Value: "ycsb-a",
					},
					&cli.StringFlag{
						Name:  "crypto",
						Usage: "Cryptographic scheme (ed25519, mac)",
						Value: "ed25519",
					},
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		panic(err)
	}
}
