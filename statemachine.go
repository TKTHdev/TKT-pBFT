package main

import ()

func (p *PBFT) applyCommandLocked(command []byte) string {
	commandStr := string(command)
	parts := splitCommand(commandStr)
	if len(parts) == 0 {
		return "Invalid Command"
	}
	switch parts[0] {
	case "SET":
		if len(parts) != 3 {
			return "Invalid SET"
		}
		key := parts[1]
		value := parts[2]
		p.StateMachine[key] = value
		p.logPutLocked("Applied command to state machine: "+commandStr, GREEN)
		return "OK"

	case "GET":
		if len(parts) != 2 {
			return "Invalid GET"
		}
		key := parts[1]
		val, ok := p.StateMachine[key]
		if !ok {
			return "Key not found"
		}
		return val

	case "DELETE":
		if len(parts) != 2 {
			return "Invalid DELETE"
		}
		key := parts[1]
		delete(p.StateMachine, key)
		p.logPutLocked("Applied command to state machine: "+commandStr, GREEN)
		return "OK"
	default:
		return "Unknown command"
	}
}

func splitCommand(command string) []string {
	var parts []string
	current := ""
	for i := 0; i < len(command); i++ {
		if command[i] == ' ' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(command[i])
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
