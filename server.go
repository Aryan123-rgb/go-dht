package main

import (
	"context"
	"log"
	"sync"

	"github.com/Aryan123-rgb/go-dht/proto"
)

/*
This file defines the server structure and 4 RPC handlers for inter - node communication:

1. DHTServer:
   The concrete implementation of the generated gRPC DHT service.
   It embeds proto.UnimplementedDHTServer for forward compatibility
   and contains the full runtime state of a node, including:
     - Self:        The local node’s identity (ID + address).
     - RoutingTable:The Kademlia-style routing table used for node lookup.
     - Storage:     A local in-memory key-value store.
     - storageMu:   A mutex protecting concurrent access to Storage.

2. RPC Handlers:
   The four core DHT RPC methods used for inter-node communication:

   - Ping:
     Used for liveness checks and node discovery. The sender is added
     to the routing table, and the local node identity is returned.

   - Store:
     Stores a key-value pair locally. The sender is recorded in the
     routing table, and the value is saved in the node’s storage.
     Thread safety is ensured using a mutex.

   - FindNode:
     Returns the closest known nodes to a target ID. The sender is
     recorded to support continuous network discovery.

   - FindValue:
     Looks up a key in local storage. If found, the value is returned.
     Otherwise, the closest known nodes are returned to allow the
     requester to continue the lookup process.

Node Discovery Behavior:
Every incoming RPC request includes the sender’s node information.
On receipt, the sender is inserted into the routing table. This
ensures passive network discovery and gradual routing table population
as nodes communicate.

Overall, this file contains the networking layer of the DHT node:
it handles RPC communication, maintains routing knowledge, and
manages local key-value storage.
*/

type DHTServer struct {
	proto.UnimplementedDHTServer
	Self         *proto.Node
	RoutingTable *RoutingTable
	Storage      map[string][]byte
	storageMu    sync.Mutex
}

// NewDHTServer initializes the server state
func NewDHTServer(self *proto.Node, rt *RoutingTable) *DHTServer {
	return &DHTServer{
		Self:         self,
		RoutingTable: rt,
		Storage:      make(map[string][]byte),
	}
}

// Ping responds to a ping and adds the sender to the routing table
func (s *DHTServer) Ping(ctx context.Context, req *proto.PingRequest) (*proto.PingResponse, error) {
	if req.Sender != nil {
		s.RoutingTable.AddNode(req.Sender)
	}
	return &proto.PingResponse{
		Sender: s.Self,
	}, nil
}

// Store saves a key-value pair to a local storage and logs the action
func (s *DHTServer) Store(ctx context.Context, req *proto.StoreRequest) (*proto.StoreResponse, error) {
	if req.Sender != nil {
		s.RoutingTable.AddNode(req.Sender)
	}
	s.storageMu.Lock()
	s.Storage[req.Key] = req.Value
	s.storageMu.Unlock()

	// Log the storage action so you can verify it in the terminal
	log.Printf("[STORE] Node %x saved Key: '%s', Value: '%s' (Instructed by %x)",
		s.Self.Id[:4], req.Key, string(req.Value), req.Sender.Id[:4])

	return &proto.StoreResponse{
		Success: true,
	}, nil
}

// FindNode processes discovery requests and returns the closest known nodes
func (s *DHTServer) FindNode(ctx context.Context, req *proto.FindNodeRequest) (*proto.FindNodeResponse, error) {
	if req.Sender != nil {
		s.RoutingTable.AddNode(req.Sender)
	}
	nodes := s.RoutingTable.GetClosestNode()
	nodes = append(nodes, s.Self)
	return &proto.FindNodeResponse{
		Nodes: nodes,
	}, nil
}

// FindValue looks up a key locally, if the key is found it returns 
// the value of key, if not found it returns the closest node to continue the discovery
func (s *DHTServer) FindValue(ctx context.Context, req *proto.FindValueRequest) (*proto.FindValueResponse, error) {
	if req.Sender != nil {
		s.RoutingTable.AddNode(req.Sender)
	}

	log.Printf("[RETRIEVE] Node %x received request for Key: '%s' (from %x)",
		s.Self.Id[:4], req.Key, req.Sender.Id[:4])

	s.storageMu.Lock()
	val, isExists := s.Storage[req.Key]
	s.storageMu.Unlock()

	if isExists {
		log.Printf("[RETRIEVE] Key '%s' found locally! Returning data.", req.Key)
		return &proto.FindValueResponse{
			Value: val,
			Found: true,
		}, nil
	}
	log.Printf("[RETRIEVE] Key '%s' not found locally. Returning closest known nodes.", req.Key)
	nodes := s.RoutingTable.GetClosestNode()
	return &proto.FindValueResponse{
		Nodes: nodes,
		Found: false,
	}, nil
}
