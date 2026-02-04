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
		
		if ch, ok := p.pendingResponses[seq]; ok {
			delete(p.pendingResponses, seq)
			resp := Response{
				success: true,
				value:   value,
			}
			select {
			case ch <- resp:
			default:
			}
		}
	}
}
