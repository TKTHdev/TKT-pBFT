package main

import (
	"fmt"
)

const (
	RPCPrePrepare  = "PBFT.PrePrepare"
	RPCPrepare     = "PBFT.Prepare"
	RPCCommit      = "PBFT.Commit"
	RPCClientReply = "PBFT.ClientReply"
)

type PrePrepareArgs struct {
	View           int
	SequenceNumber int
	Digest         string
	Command        []byte
	Signature      []byte
}

type PrePrepareReply struct {
	Success bool
}

type PrepareArgs struct {
	View           int
	SequenceNumber int
	Digest         string
	NodeID         int
	Signature      []byte
}

type PrepareReply struct {
	Success bool
}

type CommitArgs struct {
	View           int
	SequenceNumber int
	Digest         string
	NodeID         int
	Signature      []byte
}

type CommitReply struct {
	Success bool
}

type ClientReplyArgs struct {
	SequenceNumber int
	NodeID         int
	Value          string
}

type ClientReplyReply struct {
	Success bool
}

func (p *PBFT) PrePrepare(args *PrePrepareArgs, reply *PrePrepareReply) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 0. Verify Signature
	// In PrePrepare, the sender is the Primary.
	// Primary ID depends on View.
	primaryID := (args.View % p.clusterSize) + 1
	var verifyKey interface{}
	if p.cryptoType == CryptoMAC {
		verifyKey = p.macKeys[primaryID]
	} else {
		verifyKey = p.pubKeys[primaryID]
	}
	if verifyKey == nil {
		reply.Success = false
		return nil
	}
	data := digestPrePrepare(args.View, args.SequenceNumber, args.Digest)
	if err := verify(verifyKey, data, args.Signature); err != nil {
		p.logPutLocked(fmt.Sprintf("Signature verification failed for PrePrepare seq %d from %d. SigLen: %d", args.SequenceNumber, primaryID, len(args.Signature)), RED)
		reply.Success = false
		return nil
	}

	// 1. Check view
	if args.View != p.view {
		reply.Success = false
		return nil
	}

	// 2. Accept and store
	state := p.getRequestState(args.SequenceNumber)
	if state.PrePrepared {
		// Already received
		reply.Success = true
		return nil
	}

	state.PrePrepared = true
	state.PrePrepareMsg = args

	// WAL
	if err := p.storage.AppendEntry(LogEntry{View: args.View, Command: args.Command}); err != nil {
		p.logPutLocked("Failed to append to log", RED)
		reply.Success = false
		return nil
	}

	p.logPutLocked(fmt.Sprintf("Received PrePrepare for seq %d", args.SequenceNumber), BLUE)

	// 3. Broadcast Prepare
	go p.broadcastPrepare(args.View, args.SequenceNumber, args.Digest)

	// Add own prepare to state
	state.PrepareMsgs[p.id] = true

	reply.Success = true
	return nil
}

func (p *PBFT) Prepare(args *PrepareArgs, reply *PrepareReply) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 0. Verify Signature
	var verifyKey interface{}
	if p.cryptoType == CryptoMAC {
		verifyKey = p.macKeys[args.NodeID]
	} else {
		verifyKey = p.pubKeys[args.NodeID]
	}
	if verifyKey == nil {
		reply.Success = false
		return nil
	}
	data := digestPrepare(args.View, args.SequenceNumber, args.Digest, args.NodeID)
	if err := verify(verifyKey, data, args.Signature); err != nil {
		p.logPutLocked(fmt.Sprintf("Signature verification failed for Prepare from %d seq %d. SigLen: %d", args.NodeID, args.SequenceNumber, len(args.Signature)), RED)
		reply.Success = false
		return nil
	}

	if args.View != p.view {
		reply.Success = false
		return nil
	}

	state := p.getRequestState(args.SequenceNumber)
	state.PrepareMsgs[args.NodeID] = true

	p.logPutLocked(fmt.Sprintf("Received Prepare from %d for seq %d (Count: %d)", args.NodeID, args.SequenceNumber, len(state.PrepareMsgs)), YELLOW)

	p.checkPreparedLocked(state, args.SequenceNumber, args.Digest)

	reply.Success = true
	return nil
}

func (p *PBFT) Commit(args *CommitArgs, reply *CommitReply) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 0. Verify Signature
	var verifyKey interface{}
	if p.cryptoType == CryptoMAC {
		verifyKey = p.macKeys[args.NodeID]
	} else {
		verifyKey = p.pubKeys[args.NodeID]
	}
	if verifyKey == nil {
		reply.Success = false
		return nil
	}
	data := digestCommit(args.View, args.SequenceNumber, args.Digest, args.NodeID)
	if err := verify(verifyKey, data, args.Signature); err != nil {
		p.logPutLocked(fmt.Sprintf("Signature verification failed for Commit from %d seq %d. SigLen: %d", args.NodeID, args.SequenceNumber, len(args.Signature)), RED)
		reply.Success = false
		return nil
	}

	if args.View != p.view {
		reply.Success = false
		return nil
	}

	state := p.getRequestState(args.SequenceNumber)
	state.CommitMsgs[args.NodeID] = true

	p.logPutLocked(fmt.Sprintf("Received Commit from %d for seq %d (Count: %d)", args.NodeID, args.SequenceNumber, len(state.CommitMsgs)), ORANGE)

	p.checkCommittedLocked(state, args.SequenceNumber, args.Digest)

	reply.Success = true
	return nil
}

func (p *PBFT) getRequestState(seq int) *RequestState {
	if _, ok := p.reqState[seq]; !ok {
		p.reqState[seq] = &RequestState{
			PrepareMsgs:   make(map[int]bool),
			CommitMsgs:    make(map[int]bool),
			ClientReplies: make(map[int]string),
		}
	}
	return p.reqState[seq]
}
