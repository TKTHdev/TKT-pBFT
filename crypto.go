package main

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	mrand "math/rand"
)

// CryptoType represents the authentication scheme
type CryptoType string

const (
	CryptoEd25519 CryptoType = "ed25519"
	CryptoMAC     CryptoType = "mac"
)

// DeterministicReader is a dummy reader for deterministic key generation (FOR TESTING ONLY)
// In production, use crypto/rand.Reader
type DeterministicReader struct {
	src mrand.Source
}

func (r *DeterministicReader) Read(p []byte) (n int, err error) {
	for i := 0; i < len(p); i++ {
		p[i] = byte(r.src.Int63())
	}
	return len(p), nil
}

func generateEd25519Key(id int) (ed25519.PrivateKey, error) {
	// Use ID as seed to generate same key for same ID every time
	src := mrand.NewSource(int64(id + 1000))
	reader := &DeterministicReader{src: src}
	_, priv, err := ed25519.GenerateKey(reader)
	return priv, err
}

func generateMACKey(id1, id2 int) []byte {
	// Generate a shared key for a pair of nodes
	// Order-independent: key(1,2) == key(2,1)
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	src := mrand.NewSource(int64(id1*1000 + id2))
	reader := &DeterministicReader{src: src}
	key := make([]byte, 32)
	reader.Read(key)
	return key
}

func sign(key interface{}, data []byte) ([]byte, error) {
	switch k := key.(type) {
	case ed25519.PrivateKey:
		return ed25519.Sign(k, data), nil
	case []byte:
		h := hmac.New(sha256.New, k)
		h.Write(data)
		return h.Sum(nil), nil
	default:
		return nil, fmt.Errorf("unknown key type for signing")
	}
}

func verify(key interface{}, data []byte, signature []byte) error {
	switch k := key.(type) {
	case ed25519.PublicKey:
		if ed25519.Verify(k, data, signature) {
			return nil
		}
		return fmt.Errorf("invalid signature")
	case []byte:
		h := hmac.New(sha256.New, k)
		h.Write(data)
		expected := h.Sum(nil)
		if hmac.Equal(expected, signature) {
			return nil
		}
		return fmt.Errorf("invalid mac")
	default:
		return fmt.Errorf("unknown key type for verification")
	}
}

// Helper to construct data for signing
func digestPrePrepare(view int, seq int, digest string) []byte {
	return []byte(fmt.Sprintf("%d:%d:%s", view, seq, digest))
}

func digestPrepare(view int, seq int, digest string, nodeID int) []byte {
	return []byte(fmt.Sprintf("%d:%d:%s:%d", view, seq, digest, nodeID))
}

func digestCommit(view int, seq int, digest string, nodeID int) []byte {
	return []byte(fmt.Sprintf("%d:%d:%s:%d", view, seq, digest, nodeID))
}
