# Almost vibe coding implemented pbft
[Japanese](README.ja.md)
This repository contains a simple implementation of the Practical Byzantine Fault Tolerance (PBFT) consensus algorithm in Go. It was implemented primarily through "vibe coding" (interactive coding with an LLM).

---

## 🚀 How to Run

The project uses a `makefile` to automate building, deploying, and running the cluster.

### Prerequisites
- Go 1.22+
- `make`
- `jq` (for parsing config)
- SSH access to the nodes (if running on multiple machines)
- `cluster.conf` configured with node IPs

### Commands

1.  **Build the project**
    ```bash
    make build
    # or for local development
    go build -o pbft_server .
    ```

2.  **Start the cluster**
    This command starts the PBFT nodes on the machines defined in `cluster.conf`.
    ```bash
    make start
    ```

3.  **Run Benchmark**
    Runs the YCSB-like benchmark scripts.
    ```bash
    make benchmark
    ```

4.  **Stop the cluster**
    ```bash
    make kill
    ```

5.  **Clean logs and binaries**
    ```bash
    make clean
    ```

---

## 🚧 Unimplemented Parts

Although the normal case operation (PrePrepare -> Prepare -> Commit) works, several critical components of a production-ready PBFT are missing:

1.  **View Change Protocol**
    -   The current implementation assumes a stable primary leader. There is no logic to detect leader failure (timeouts) or switch to a new view (ViewChange/NewView messages).

2.  **Checkpointing & Log Garbage Collection**
    -   The log grows indefinitely. There is no checkpoint mechanism to truncate the log and discard old entries.

3.  **State Transfer**
    -   Nodes that fall behind cannot fetch missing state/logs from other peers to catch up.

4.  **Robust Client**
    -   The client logic is currently embedded in the primary node for benchmarking purposes. A proper external client that handles request timeouts and redirects to the new leader is not implemented.

5.  **Dynamic Membership**
    -   The cluster size is static and defined in `cluster.conf`.
