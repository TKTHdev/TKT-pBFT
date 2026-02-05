package main

import (
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

	PrePrepareMsg *PrePrepareArgs
	PrepareMsgs   map[int]bool // NodeID -> bool
	CommitMsgs    map[int]bool // NodeID -> bool

	// Track replies for client verification
	ClientReplies map[int]string // NodeID -> Value
	ReplySent     bool           // True if we already sent response to client
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
	cryptoType CryptoType
	privKey    interface{}            // ed25519.PrivateKey or nil for MAC
	pubKeys    map[int]interface{}    // ed25519.PublicKey for ed25519, []byte for MAC (shared key with peer)
	macKeys    map[int][]byte         // MAC: shared keys with each peer

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
	
	pendingResponses map[int][]chan Response // SequenceNumber -> Response Channels

	// Client Handling
	mu sync.RWMutex
}

func NewPBFT(id int, confPath string, writeBatchSize int, readBatchSize int, workers int, debug bool, workload int, asyncLog bool, inMemory bool, cryptoType CryptoType) *PBFT {
	peerIPPort := parseConfig(confPath)

	storage, err := NewStorage(id, asyncLog, inMemory)
	if err != nil {
		panic(err)
	}

	// Generate Keys based on crypto type
	var privKey interface{}
	pubKeys := make(map[int]interface{})
	macKeys := make(map[int][]byte)

	switch cryptoType {
	case CryptoEd25519:
		pk, err := generateEd25519Key(id)
		if err != nil {
			panic(err)
		}
		privKey = pk
		for peerID := range peerIPPort {
			key, err := generateEd25519Key(peerID)
			if err != nil {
				panic(err)
			}
			pubKeys[peerID] = key.Public()
		}
	case CryptoMAC:
		privKey = nil
		for peerID := range peerIPPort {
			sharedKey := generateMACKey(id, peerID)
			macKeys[peerID] = sharedKey
			pubKeys[peerID] = sharedKey // For verification in RPC handlers
		}
	default:
		panic(fmt.Sprintf("unknown crypto type: %s", cryptoType))
	}

	p := &PBFT{
		id:               id,
		confPath:         confPath,
		writeBatchSize:   writeBatchSize,
		readBatchSize:    readBatchSize,
		workers:          workers,
		debug:            debug,
		workload:         workload,
		asyncLog:         asyncLog,
		peerIPPort:       peerIPPort,
		clusterSize:      len(peerIPPort),
		rpcConns:         make(map[int]*rpc.Client),
		cryptoType:       cryptoType,
		privKey:          privKey,
		pubKeys:          pubKeys,
		macKeys:          macKeys,
		view:             0,
		sequenceNumber:   0,
		reqState:         make(map[int]*RequestState),
		storage:          storage,
		StateMachine:     make(map[string]string),
		ReqCh:            make(chan ClientRequest, 5000),
		ReadCh:           make(chan []ClientRequest, 500),
		pendingResponses: make(map[int][]chan Response),
		mu:               sync.RWMutex{},
	}
	fmt.Println(p)

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
