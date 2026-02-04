package main

import (
	"crypto/rsa"
	"fmt"
	"net/rpc"
	"sync"
)

type ClientRequest struct {
	Command []byte
	RespCh  chan Response
}

type LogEntry struct {
	View    int
	Command []byte
}

type RequestState struct {
	PrePrepared bool
	Prepared    bool
	Committed   bool
	
	PrePrepareMsg  *PrePrepareArgs
	PrepareMsgs    map[int]bool // NodeID -> bool
	CommitMsgs     map[int]bool // NodeID -> bool
	
	// Track replies for client verification
	ClientReplies  map[int]string // NodeID -> Value
	ReplySent      bool           // True if we already sent response to client
}

type PBFT struct {
	id             int
	confPath       string
	writeBatchSize int
	readBatchSize  int
	workers        int
	debug          bool
	workload       int
	asyncLog       bool

	// Network and Cluster
	peerIPPort  map[int]string
	clusterSize int
	rpcConns    map[int]*rpc.Client

	// Crypto
	privKey *rsa.PrivateKey
	pubKeys map[int]*rsa.PublicKey

	// Consensus State
	view           int
	sequenceNumber int
	reqState       map[int]*RequestState // SequenceNumber -> State
	
	// Storage & State Machine
	storage      *Storage
	StateMachine map[string]string
	
	// Communication
	ReqCh  chan ClientRequest
	ReadCh chan []ClientRequest
	
	pendingResponses map[int]chan Response // SequenceNumber -> Response Channel

	// Client Handling
	mu sync.RWMutex
}

func NewPBFT(id int, confPath string, writeBatchSize int, readBatchSize int, workers int, debug bool, workload int, asyncLog bool) *PBFT {
	peerIPPort := parseConfig(confPath)

	storage, err := NewStorage(id, asyncLog)
	if err != nil {
		panic(err)
	}
	
	// Generate Keys
	privKey, err := generateKey(id)
	if err != nil {
		panic(err)
	}
	pubKeys := make(map[int]*rsa.PublicKey)
	for peerID := range peerIPPort {
		key, err := generateKey(peerID)
		if err != nil {
			panic(err)
		}
		pubKeys[peerID] = &key.PublicKey
	}

	p := &PBFT{
		id:             id,
		confPath:       confPath,
		writeBatchSize: writeBatchSize,
		readBatchSize:  readBatchSize,
		workers:        workers,
		debug:          debug,
		workload:       workload,
		asyncLog:       asyncLog,
		peerIPPort:     peerIPPort,
		clusterSize:    len(peerIPPort),
		rpcConns:       make(map[int]*rpc.Client),
		privKey:        privKey,
		pubKeys:        pubKeys,
		view:           0, 
		sequenceNumber: 0,
		reqState:       make(map[int]*RequestState),
		storage:        storage,
		StateMachine:   make(map[string]string),
		ReqCh:          make(chan ClientRequest, 5000),
		ReadCh:         make(chan []ClientRequest, 500),
		pendingResponses: make(map[int]chan Response),
		mu:             sync.RWMutex{},
	}

	return p
}

func (p *PBFT) Run() {
	fmt.Printf("PBFT node %d starting... (Cluster Size: %d)\n", p.id, p.clusterSize)

	go p.listenRPC()
	p.dialRPCToAllPeers()
	
	go p.concClient()
	go p.handleClientRequest()

	select {}
}

func (p *PBFT) isPrimary() bool {
	// Primary is usually view % N
	// IDs are 1-based in config (1, 2, 3, 4)
	// (view % N) + 1 == id
	return (p.view%p.clusterSize)+1 == p.id
}