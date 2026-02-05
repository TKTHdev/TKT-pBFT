USER         := tkt
PROJECT_DIR  := ~/proj/pbft
BINARY_NAME  := pbft_server
CONFIG_FILE  := cluster.conf
LOG_DIR      := $(PROJECT_DIR)/logs

ALL_IDS := $(shell jq -r '.[].id' $(CONFIG_FILE) 2>/dev/null || echo "")
TARGET_ID ?= all
ifeq ($(TARGET_ID),all)
    IDS := $(ALL_IDS)
else
    IDS := $(TARGET_ID)
endif

DEBUG ?= false
DEBUG_FLAG := 
ifeq ($(DEBUG),true)
    DEBUG_FLAG := --debug
endif

ASYNC_LOG ?= false
ASYNC_FLAG := 
ifeq ($(ASYNC_LOG),true)
    ASYNC_FLAG := --async-log
endif

IN_MEMORY ?= false
MEMORY_FLAG := 
ifeq ($(IN_MEMORY),true)
    MEMORY_FLAG := --in-memory
endif

ARGS ?= 

WORKERS ?= 1 2 4 8 16 32
READ_BATCH ?= 1 2 4 8 16 32
WRITE_BATCH ?= 1 2 4 8 16 32
TYPE    ?= ycsb-a
TIMESTAMP := $(shell date +%Y%m%d_%H%M%S)

.PHONY: help deploy build send-bin start kill clean benchmark

help:
	@echo "Usage: make [target] [TARGET_ID=id] [DEBUG=true] [ASYNC_LOG=true] [IN_MEMORY=true]"
	@echo "Targets: deploy, build, send-bin, start, kill, clean, benchmark"


deploy:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		echo "[$$ip] Distributing config..."; \
		scp $(CONFIG_FILE) $(USER)@$$ip:$(PROJECT_DIR)/$(notdir $(CONFIG_FILE)) & \
	done; wait

build:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Building $$bin..."; \
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && go build -o $$bin" & \
	done; wait

send-bin:
	GOOS=linux GOARCH=amd64 go build -o /tmp/pbft_tmp .
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Sending $$bin..."; \
		scp /tmp/pbft_tmp $(USER)@$$ip:$(PROJECT_DIR)/$$bin && \
		ssh $(USER)@$$ip "chmod +x $(PROJECT_DIR)/$$bin" & \
	done; wait
	@rm /tmp/pbft_tmp

start:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Starting $$bin (ID: $$id)..."; \
		ssh -n -f $(USER)@$$ip "mkdir -p $(LOG_DIR) && cd $(PROJECT_DIR) && \
		   (pkill -x $$bin || true) && \
		   sleep 0.5 && \
		   nohup ./$$bin start --id $$id --conf cluster.conf $(ARGS) $(DEBUG_FLAG) $(ASYNC_FLAG) $(MEMORY_FLAG) > $(LOG_DIR)/node_$$id.ans 2>&1 < /dev/null &"; \
	done
	@echo "All start commands initiated."

kill:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Killing $$bin..."; \
		ssh $(USER)@$$ip "pkill -x $$bin || echo 'Not running.'" & \
	done; wait

clean:
	@for id in $(IDS); do \
		ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
		bin="$(BINARY_NAME)_$$id"; \
		echo "[$$ip] Cleaning $$bin..."; \
		ssh $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f $$bin logs/node_$$id.ans *.bin /dev/shm/pbft_*.bin" results/* & \
	done; wait

benchmark:
	@mkdir -p results
	@echo "Starting benchmark..."
	@for type in $(TYPE); do \
		BENCH_FILE="results/benchmark-$(TIMESTAMP)-$$type.csv"; \
		echo "Initializing $$BENCH_FILE ..."; \
		echo "Workload,ReadBatch,WriteBatch,Workers,Throughput(ops/sec),Latency(ms)" > "$$BENCH_FILE"; \
		\
		for rbatch in $(READ_BATCH); do \
			for wbatch in $(WRITE_BATCH); do \
				for workers in $(WORKERS); do \
					echo "Running benchmark: Type=$$type, ReadBatch=$$rbatch, WriteBatch=$$wbatch, Workers=$$workers"; \
					\
					for id in $(IDS); do \
						ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
						ssh -n $(USER)@$$ip "rm -f $(LOG_DIR)/node_$$id.ans"; \
						if [ "$$type" != "ycsb-c" ]; then \
							ssh -n $(USER)@$$ip "cd $(PROJECT_DIR) && rm -f pbft_log_$$id.bin pbft_state_$$id.bin /dev/shm/pbft_log_$$id.bin /dev/shm/pbft_state_$$id.bin"; \
						fi; \
					done; \
					\
					$(MAKE) kill; \
					sleep 2; \
					$(MAKE) start ARGS="--read-batch-size $$rbatch --write-batch-size $$wbatch --workers $$workers --workload $$type $(ASYNC_FLAG) $(MEMORY_FLAG)"; \
					sleep 20; \
					\
					echo "--- Collecting results for Type=$$type, Workers=$$workers ---"; \
					\
					for id in $(IDS); do \
						ip=$$(jq -r --arg i "$$id" '.[] | select(.id == ($$i | tonumber)) | .ip' $(CONFIG_FILE)); \
						\
						RES=$$(ssh -n $(USER)@$$ip "grep 'RESULT:' $(LOG_DIR)/node_$$id.ans | tail -n 1" | sed 's/.*RESULT://' | awk -F, '{print $$(NF-1) "," $$NF}' | tr -d ' \r\n'); \
						\
						if [ -n "$$RES" ]; then \
							echo "$$type,$$rbatch,$$wbatch,$$workers,$$RES" >> "$$BENCH_FILE"; \
						fi; \
					done; \
				done; \
			done; \
		done; \
		echo "Finished workload: $$type. Results saved to $$BENCH_FILE"; \
	done
	@$(MAKE) kill
	@echo "All benchmarks finished."
