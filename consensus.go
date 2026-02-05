package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func (p *PBFT) broadcastPrePrepare(seq int, command []byte) {
	p.mu.Lock()
	view := p.view
	digest := hash(command)

	// Sign
	data := digestPrePrepare(view, seq, digest)
	sig, err := sign(p.privKey, data)
	if err != nil {
		p.logPutLocked("Error signing PrePrepare", RED)
		p.mu.Unlock()
		return
	}

	args := &PrePrepareArgs{
		View:           view,
		SequenceNumber: seq,
		Digest:         digest,
		Command:        command,
		Signature:      sig,
	}

	// Store own state
	state := p.getRequestState(seq)
	state.PrePrepared = true
	state.PrePrepareMsg = args

	// WAL
	if err := p.storage.AppendEntry(LogEntry{View: view, Command: command}); err != nil {
		p.logPutLocked("Failed to append to log", RED)
		p.mu.Unlock()
		return
	}

	p.mu.Unlock()

	p.logPut(fmt.Sprintf("Broadcasting PrePrepare for seq %d", seq), BLUE)

	for peerID := range p.peerIPPort {
		if peerID != p.id {
			go func(target int) {
				reply := &PrePrepareReply{}
				p.sendRPC(target, RPCPrePrepare, args, reply)
			}(peerID)
		}
	}

}

func (p *PBFT) broadcastPrepare(view int, seq int, digest string) {
	// Sign (need privKey, can access if immutable, or lock)
	// Ideally sign outside lock, but we need lock for other things potentially?
	// Here we are called from goroutine or handlePrePrepare.
	// If called from handlePrePrepare, lock is NOT held (it uses 'go broadcastPrepare').

	data := digestPrepare(view, seq, digest, p.id)
	sig, err := sign(p.privKey, data)
	if err != nil {
		p.logPut("Error signing Prepare", RED)
		return
	}

	args := &PrepareArgs{
		View:           view,
		SequenceNumber: seq,
		Digest:         digest,
		NodeID:         p.id,
		Signature:      sig,
	}

	for peerID := range p.peerIPPort {
		if peerID != p.id {
			go func(target int) {
				reply := &PrepareReply{}
				p.sendRPC(target, RPCPrepare, args, reply)
			}(peerID)
		}
	}
}

func (p *PBFT) broadcastCommit(view int, seq int, digest string) {
	data := digestCommit(view, seq, digest, p.id)
	sig, err := sign(p.privKey, data)
	if err != nil {
		p.logPut("Error signing Commit", RED)
		return
	}

	args := &CommitArgs{
		View:           view,
		SequenceNumber: seq,
		Digest:         digest,
		NodeID:         p.id,
		Signature:      sig,
	}

	for peerID := range p.peerIPPort {
		if peerID != p.id {
			go func(target int) {
				reply := &CommitReply{}
				p.sendRPC(target, RPCCommit, args, reply)
			}(peerID)
		}
	}
}

func (p *PBFT) checkPreparedLocked(state *RequestState, seq int, digest string) {
	if state.Prepared {
		return
	}

	// We need PrePrepare
	if !state.PrePrepared {
		return
	}

	// We need 2f Prepares from *other* replicas.
	// Actually, the condition is 2f+1 matches (including PrePrepare).
	// Since we are consistent, let's just count how many unique nodes agreed (PrePrepare sender + Prepare senders).
	// But PrePrepare sender is Primary.

	f := (p.clusterSize - 1) / 3
	quorum := 2 * f // We need 2f Prepares because PrePrepare counts as 1.

	if len(state.PrepareMsgs) >= quorum {
		state.Prepared = true
		p.logPutLocked(fmt.Sprintf("Seq %d Prepared (Quorum %d). Broadcasting Commit.", seq, quorum), GREEN)

		// Add own Commit
		state.CommitMsgs[p.id] = true
		go p.broadcastCommit(p.view, seq, digest)

		// Check if we can commit immediately (if we already received enough commits)
		p.checkCommittedLocked(state, seq, digest)
	}
}

func (p *PBFT) checkCommittedLocked(state *RequestState, seq int, digest string) {
	if state.Committed {
		return
	}

	if !state.Prepared {
		return
	}

	f := (p.clusterSize - 1) / 3
	quorum := 2*f + 1

	if len(state.CommitMsgs) >= quorum {
		state.Committed = true
		p.logPutLocked(fmt.Sprintf("Seq %d Committed (Quorum %d). Executing.", seq, quorum), GREEN)

		// Execute
		if state.PrePrepareMsg != nil {
			p.executeLocked(seq, state.PrePrepareMsg.Command)
		}
	}
}

func (p *PBFT) executeLocked(seq int, command []byte) {
	// Apply to State Machine
	// Try to decode as batch. If it fails (e.g. single command from older version or test), fallback?
	// But we changed processWriteBatch to always pack.
	// So we assume it is a batch.
	
	cmds, err := decodeBatch(command)
	var results []string
	
	if err != nil {
		// Fallback for non-batched legacy/test? 
		// Or assume it's just one command raw?
		// For safety in this refactor, let's assume if decode fails, it's a single command
		// But decodeBatch checks length prefix, so a raw string might look like garbage length.
		// Let's assume strict batching for now as we control the sender.
		// Actually, let's treat it as a single command if decode fails to be safe?
		// No, decodeBatch might read random bytes as length and panic or allocate huge memory.
		// Better to be strict. But wait, what if decodeBatch returns error?
		// Since we implemented encodeBatch, we expect valid batch.
		// Let's log error and try treating as single command?
		// Actually, given we changed the sender, we should expect batch format.
		// But let's wrap in try-catch logic effectively by checking err.
		if err != nil {
			p.logPutLocked("Error decoding batch, treating as single command", RED)
			val := p.applyCommandLocked(command)
			results = append(results, val)
		} else {
			for _, cmd := range cmds {
				val := p.applyCommandLocked(cmd)
				results = append(results, val)
			}
		}
	} else {
		for _, cmd := range cmds {
			val := p.applyCommandLocked(cmd)
			results = append(results, val)
		}
	}
	
	resultValue := encodeBatchResults(results)

	if p.isPrimary() {
		// Primary is local to the client in this simulation.
		// So it treats its own execution as one of the replies.
		p.handleClientReplyLocked(seq, p.id, resultValue)
	} else {
		// Backup nodes send their reply to the Primary (who hosts the client)
		// Find Primary ID
		primaryID := (p.view % p.clusterSize) + 1

		args := &ClientReplyArgs{
			SequenceNumber: seq,
			NodeID:         p.id,
			Value:          resultValue,
		}

		go func(target int, a *ClientReplyArgs) {
			reply := &ClientReplyReply{}
			p.sendRPC(target, RPCClientReply, a, reply)
		}(primaryID, args)
	}
}

func (p *PBFT) sendRPC(peerID int, method string, args interface{}, reply interface{}) bool {
	p.mu.Lock()
	client := p.rpcConns[peerID]
	p.mu.Unlock()

	if client == nil {
		// Try to connect (simple retry logic)
		p.dialRPCToPeer(peerID)
		p.mu.Lock()
		client = p.rpcConns[peerID]
		p.mu.Unlock()
		if client == nil {
			return false
		}
	}

	err := client.Call(method, args, reply)
	if err != nil {
		// p.logPut(fmt.Sprintf("RPC %s to %d failed: %v", method, peerID, err), PURPLE)
		return false
	}
	return true
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
