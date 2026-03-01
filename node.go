package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"log"
	"math/bits"
	"sync"

	"github.com/Aryan123-rgb/go-dht/proto"
)

const IDLength = 20
const MaxBuckets = IDLength * 8 // 160 buckets for 160-bit IDs

// NodeId represents the 160-bit DHT Id
type NodeId [IDLength]byte

// GenerateId creates a SHA-1 hash for node addresses or data keys
func GenerateID(data string) NodeId {
	return sha1.Sum([]byte(data))
}

// BucketIndex calculates the highest differing bit between two IDs (0-159), this determines which bucket a node belongs in
func BucketIndex(id1, id2 NodeId) int {
	for i := range IDLength {
		xor := id1[i] ^ id2[i]
		if xor != 0 {
			// find the highest set bit in this differing byte
			leadingZeros := bits.LeadingZeros8(xor)
			return (IDLength-1-i)*8 + (8 - 1 - leadingZeros)
		}
	}
	return 0 // IDs are identical
}

// RoutingTable manages the 160 buckets of known network nodes
type RoutingTable struct {
	mu      sync.Mutex
	SelfId  NodeId
	Buckets [MaxBuckets][]*proto.Node
}

// NewRoutingTable initializes an empty routing table
func NewRoutingTable(selfId NodeId) *RoutingTable {
	return &RoutingTable{
		SelfId: selfId,
	}
}

// AddNode processes any encountered node. If it is new, it adds it to the correct bucket based on XOR distance and logs the discovery
func (rt *RoutingTable) AddNode(node *proto.Node) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var id NodeId
	copy(id[:], node.Id)

	// Ignore ourselves
	if id == rt.SelfId {
		return
	}

	bucketIdx := BucketIndex(rt.SelfId, id)

	// Check if we already know this node
	for _, n := range rt.Buckets[bucketIdx] {
		if bytes.Equal(n.Id, node.Id) {
			return
		}
	}

	// Trigger the discovery log for the terminal
	log.Printf("[DISCOVERY] Node %s discovered new node %s at %s (Bucket: %d)",
		rt.SelfId, hex.EncodeToString(node.Id)[:8], node.IpAddress, bucketIdx)

	// Append to the bucket (minimal implementation: no capacity limits or ping checks)
	rt.Buckets[bucketIdx] = append(rt.Buckets[bucketIdx], node)
}
