package main

import (
	"fmt"
)

// ClientReply handles the reply from a replica to the client (Primary acts as client proxy here)
func (p *PBFT) ClientReply(args *ClientReplyArgs, reply *ClientReplyReply) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Only Primary handles aggregation in this simulation
	if !p.isPrimary() {
		reply.Success = false
		return nil
	}

	p.handleClientReplyLocked(args.SequenceNumber, args.NodeID, args.Value)
	reply.Success = true
	return nil
}

func (p *PBFT) handleClientReplyLocked(seq int, nodeID int, value string) {
	state := p.getRequestState(seq)
	
	// Don't process if already replied to client
	if state.ReplySent {
		return
	}

	state.ClientReplies[nodeID] = value
	
	// Check for f+1 matches
	f := (p.clusterSize - 1) / 3
	required := f + 1
	
	// Count matches for this value
	count := 0
	for _, v := range state.ClientReplies {
		if v == value {
			count++
		}
	}
	
	if count >= required {
		p.logPutLocked(fmt.Sprintf("Client received %d replies for seq %d. Returning to app.", count, seq), GREEN)
		state.ReplySent = true
		
		results, err := decodeBatchResults(value)
		if err != nil {
			// If decoding fails, fallback to treating as single result?
			// This matches consensus.go's fallback logic roughly.
			p.logPutLocked("Error decoding batch results in reply", RED)
			// Try single
			results = []string{value}
		}

		if chans, ok := p.pendingResponses[seq]; ok {
			delete(p.pendingResponses, seq)
			
			// Match results to channels
			// If mismatch, we have a problem. But we assume 1:1 if batching worked.
			limit := len(chans)
			if len(results) < limit {
				limit = len(results)
			}
			
			for i := 0; i < limit; i++ {
				resp := Response{
					success: true,
					value:   results[i],
				}
				select {
				case chans[i] <- resp:
				default:
				}
			}
		}
	}
}
