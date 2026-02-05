package main

import (
	"bytes"
	"encoding/binary"
)

// Encode a batch of commands (byte slices) into a single byte slice
func encodeBatch(cmds [][]byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, int32(len(cmds)))
	for _, cmd := range cmds {
		binary.Write(&buf, binary.LittleEndian, int32(len(cmd)))
		buf.Write(cmd)
	}
	return buf.Bytes()
}

// Decode a single byte slice into a batch of commands
func decodeBatch(data []byte) ([][]byte, error) {
	buf := bytes.NewReader(data)
	var numCmds int32
	if err := binary.Read(buf, binary.LittleEndian, &numCmds); err != nil {
		return nil, err
	}

	cmds := make([][]byte, numCmds)
	for i := 0; i < int(numCmds); i++ {
		var cmdLen int32
		if err := binary.Read(buf, binary.LittleEndian, &cmdLen); err != nil {
			return nil, err
		}
		cmd := make([]byte, cmdLen)
		if _, err := buf.Read(cmd); err != nil {
			return nil, err
		}
		cmds[i] = cmd
	}
	return cmds, nil
}

// Encode a batch of results (strings) into a single string (serialized as bytes then string)
func encodeBatchResults(results []string) string {
	// Convert strings to bytes
	resultBytes := make([][]byte, len(results))
	for i, r := range results {
		resultBytes[i] = []byte(r)
	}
	encoded := encodeBatch(resultBytes)
	return string(encoded)
}

// Decode a batched result string into a slice of result strings
func decodeBatchResults(data string) ([]string, error) {
	byteResults, err := decodeBatch([]byte(data))
	if err != nil {
		return nil, err
	}
	results := make([]string, len(byteResults))
	for i, b := range byteResults {
		results[i] = string(b)
	}
	return results, nil
}
