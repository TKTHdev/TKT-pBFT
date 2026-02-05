package main

import (
	"fmt"
	"log"
	"sort"
)

const (
	BLUE = iota
	GREEN
	RED
	YELLOW
	WHITE
	CYAN
	PURPLE
	MAGENTA
	ORANGE
	EMERALD
)

func (p *PBFT) logPut(msg string, colour int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	p.logPutLocked(msg, colour)
}

func (p *PBFT) logPutLocked(msg string, colour int) {
	if !p.debug {
		return
	}
	colors := map[int]string{
		BLUE:    "\033[34m",
		GREEN:   "\033[32m",
		RED:     "\033[31m",
		YELLOW:  "\033[33m",
		WHITE:   "\033[37m",
		CYAN:    "\033[36m",
		PURPLE:  "\033[35m",
		MAGENTA: "\033[95m",
	}
	color, ok := colors[colour]
	if !ok {
		color = "\033[0m"
	}
	reset := "\033[0m"

	logPrefix := fmt.Sprintf("[Node %d | View %d] ", p.id, p.view)
	log.Printf("%s%s%s", color, logPrefix+msg, reset)
}

func (p *PBFT) printStateMachineAsStringLocked() string {
	smStr := "{"
	keys := make([]string, 0, len(p.StateMachine))
	for k := range p.StateMachine {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		v := p.StateMachine[k]
		smStr += fmt.Sprintf("%s: %s", k, v)
		if i != len(keys)-1 {
			smStr += ", "
		}
	}
	smStr += "}"
	return smStr
}
