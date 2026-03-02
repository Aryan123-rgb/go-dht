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

/*
This file defines the structure for the routing table node has.
RoutingTable will currently have 160 sized array where at each index
the nodes with differing bits will be stored

The file defines the logic of calculaing distance using xor and finding the correct bucket index based on differing bits
Also defines the logic for adding new node to the routing table and 
getting closest nodes in the routing table

TODO: 
1. Introduce the concurreny parameter alpha = 3 and only retrieve the top alpha nodes when fetching closest nodes

2. Introduce k-buckets storing and for every bucketIndex only store the top-k node and omit out the rest of the nodes (k = 20)

*/

const IDLength = 20             // 160 bits - 20 bytes
const MaxBuckets = IDLength * 8 // 160 buckets for 160-bit ids

// NodeId represents SHA-1 bit Node Id
type NodeId [IDLength]byte

// GenerateID creates a SHA-1 hash for node addresses or data keys
func GenerateID(data string) NodeId {
	return sha1.Sum([]byte(data))
}

// String returns a hex representation for clean logging
func (id NodeId) String() string {
	return hex.EncodeToString(id[:])
}

// BucketIndex calculates the highest differing bit between two IDs (0-159).
// This determines which bucket a node belongs in.
func BucketIndex(id1, id2 NodeId) int {
	for i := range IDLength {
		xor := id1[i] ^ id2[i]
		if xor != 0 {
			// Find the highest set bit in this differing byte
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

// AddNode processes any encountered node. If it is new, it adds it to 
// the correct bucket based on XOR distance and logs the discovery.
func (rt *RoutingTable) AddNode(node *proto.Node) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var id NodeId
	copy(id[:], node.Id)

	// do not add yourself
	if id == rt.SelfId {
		return
	}
	bucketIndex := BucketIndex(rt.SelfId, id)

	// check if we already know this node, avoid duplication
	for _, n := range rt.Buckets[bucketIndex] {
		if bytes.Equal(node.Id, n.Id) {
			return
		}
	}

	// Trigger the discovery log for the terminal
	log.Printf("[DISCOVERY] Node %s discovered new node %s at %s (Bucket: %d)",
		rt.SelfId.String()[:8], hex.EncodeToString(node.Id)[:8], node.IpAddress, bucketIndex)

	// Append the node to the bucket
	// TODO: Add capacity limits and ping check before adding the node
	rt.Buckets[bucketIndex] = append(rt.Buckets[bucketIndex], node)
}

// GetClosestNodes returns all known nodes from the table

// TODO: Return only alpha nodes with the shortest distance
// According to paper alpha = 3, return the first 3 nodes which are active
func (rt *RoutingTable) GetClosestNode() []*proto.Node {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var nodes []*proto.Node
	for _, n := range rt.Buckets {
		nodes = append(nodes, n...)
	}
	return nodes
}
