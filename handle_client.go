package main

import (
	"strings"
	"time"
)

const (
	READ_LINGER_TIME  = 15 * time.Millisecond
	WRITE_LINGER_TIME = 15 * time.Millisecond
)

func (p *PBFT) handleClientRequest() {
	writeBatchSize := p.writeBatchSize
	// readBatchSize := p.readBatchSize // Unused now as we treat reads as writes for consistency

	var writeReqs []ClientRequest

	var writeTimer *time.Timer
	var writeTimerCh <-chan time.Time

	flushWrites := func() {
		if len(writeReqs) > 0 {
			p.processWriteBatch(writeReqs)
			writeReqs = nil
		}
	}

	stopTimer := func(t *time.Timer) {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
	}

	for {
		select {
		case req := <-p.ReqCh:
			// Treat all requests (including GET) as write requests to ensure linearizability via consensus
			writeReqs = append(writeReqs, req)
			if len(writeReqs) >= writeBatchSize {
				flushWrites()
				if writeTimer != nil {
					stopTimer(writeTimer)
					writeTimer = nil
					writeTimerCh = nil
				}
			} else if writeTimer == nil {
				writeTimer = time.NewTimer(WRITE_LINGER_TIME)
				writeTimerCh = writeTimer.C
			}
		case <-writeTimerCh:
			flushWrites()
			writeTimer = nil
			writeTimerCh = nil
		}
	}
}

// processWriteBatch simulates handling a batch of write requests.
func (p *PBFT) processWriteBatch(reqs []ClientRequest) {
	if !p.isPrimary() {
		// If not primary, we shouldn't really be here with the current client setup.
		// Drop or forward.
		return
	}

	for _, req := range reqs {
		p.mu.Lock()
		p.sequenceNumber++
		seq := p.sequenceNumber
		p.pendingResponses[seq] = req.RespCh
		p.mu.Unlock()

		go p.broadcastPrePrepare(seq, req.Command)
	}
}

// processReadBatch simulates handling a batch of read requests.
func (p *PBFT) processReadBatch(reqs []ClientRequest) {
	// In a real PBFT, this might just be local read if strong consistency isn't required
	// or part of the protocol.
	for _, req := range reqs {
		select {
		case req.RespCh <- Response{success: true, value: "val"}:
		default:
		}
	}
}
