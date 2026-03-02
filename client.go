package main

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/Aryan123-rgb/go-dht/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client handles outbound DHT network operations
// call the gRPC server handlers, add the logic for entering the
// node in the network and bootstrapping communication, and for
// storing and retrieving the kv pair

// =======================================
// =======UTILITY FUNCTIONS===============
// =======================================
// xorDistance calculates byte-wise XOR difference between Ids
func xorDistance(id1, id2 NodeId) NodeId {
	var dist NodeId
	for i := range IDLength {
		dist[i] = id1[i] ^ id2[i]
	}
	return dist
}

// isCloser compares two distances, returns true if d1 < d2, otherwise false
func isCloser(d1, d2 NodeId) bool {
	for i := range IDLength {
		if d1[i] < d2[i] {
			return true
		} else if d1[i] > d2[i] {
			return false
		}
	}
	return false
}

// containsNode checks if a node is already present in a list of nodes
func containsNode(list []*proto.Node, target *proto.Node) bool {
	for _, n := range list {
		if string(target.Id) == string(n.Id) {
			return true
		}
	}
	return false
}

// Client handles outbout DHT network operations
type Client struct {
	Self         *proto.Node
	RoutingTable *RoutingTable
	Server       *DHTServer
}

// NewClient initilaizes a new DHT client
func NewClient(rt *RoutingTable, s *DHTServer, self *proto.Node) *Client {
	return &Client{
		Self:         self,
		RoutingTable: rt,
		Server:       s,
	}
}

// Bootstrap connects to a known node to join the network
func (c *Client) Bootstrap(bootstrapAddr string) {
	log.Printf("[NETWORK] Bootstrapping via node at %s...", bootstrapAddr)

	conn, err := grpc.NewClient(bootstrapAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("[NETWORK] Failed to connect to bootstrap node: %v", err)
		return
	}
	defer conn.Close()

	grpcClient := proto.NewDHTClient(conn)

	// Request the bootstrap node for nodes closest to our OWN ID
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := &proto.FindNodeRequest{
		Sender:   c.Self,
		TargetId: c.Self.Id,
	}

	resp, err := grpcClient.FindNode(ctx, req)
	if err != nil {
		log.Printf("[NETWORK] Bootstrap FindNode failed: %v", err)
		return
	}

	
	log.Printf("[NETWORK] Bootstrap complete. Discovered %d nodes.", len(resp.Nodes))
	// Add discovered nodes to our routing table
	for _, node := range resp.Nodes {
		c.RoutingTable.AddNode(node)
	}

}

// findClosestNodes finds the closest node in the routing table whose distance is least with the specified targetId. This is used to determine which local node will store the key-value pair
func (c *Client) findClosestNodes(targetId NodeId) *proto.Node {
	nodes := c.RoutingTable.GetClosestNode()
	if len(nodes) == 0 {
		return nil
	}

	closestNode := c.Self
	minDistance := xorDistance(NodeId(c.Self.Id), targetId)

	for _, n := range nodes {
		var nodeId NodeId
		copy(nodeId[:], n.Id)
		dist := xorDistance(nodeId, targetId)
		if isCloser(dist, minDistance) {
			closestNode = n
			minDistance = dist
		}
	}
	return closestNode
}

// StoreNetwork routes a kv pair to the closest know node and stores it in there local storage
func (c *Client) StoreNetwork(key string, value []byte) {
	// to find the closest node to the key, we convert the key to a
	// 160 bit hash Id
	targetId := GenerateID(key)
	closestNode := c.findClosestNodes(targetId)

	if closestNode == nil || string(closestNode.Id) == string(c.Self.Id) {
		log.Printf("[ROUTING] We are the closest node to Key '%s'. Storing locally.", key)
		c.Server.storageMu.Lock()
		c.Server.Storage[key] = value
		c.Server.storageMu.Unlock()
		return
	}

	// if the closest node is not the self node, make a gRPC request to that node to store the key-value pair locally
	log.Printf("[ROUTING] Routing STORE for Key '%s' to closest node %x at %s", key, closestNode.Id[:4], closestNode.IpAddress)
	conn, err := grpc.NewClient(closestNode.IpAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("[NETWORK] Dial failed: %v", err)
		return
	}
	defer conn.Close()

	grpcClient := proto.NewDHTClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := &proto.StoreRequest{
		Sender: c.Self,
		Key:    key,
		Value:  value,
	}

	resp, err := grpcClient.Store(ctx, req)
	if err != nil || !resp.Success {
		log.Printf("[NETWORK] Store RPC failed: %v", err)
	} else {
		log.Printf("[NETWORK] Successfully stored Key '%s' on node %x", key, closestNode.Id[:4])
	}
}

// FindValueNetwork retrieves a value from the network
func (c *Client) FindValueNetwork(key string) {
	// 1. Check the local storage first
	c.Server.storageMu.Lock()
	val, isExists := c.Server.Storage[key]
	c.Server.storageMu.Unlock()

	if isExists {
		log.Printf("[RETRIEVE] Found locally: %s", string(val))
		return
	}

	// 2. Initialise tracking variables
	targetId := GenerateID(key)
	queried := make(map[string]bool)
	queried[string(c.Self.Id)] = true

	shortList := c.RoutingTable.GetClosestNode()
	if len(shortList) == 0 {
		log.Printf("[RETRIEVE] Network is empty. Cannot find key.")
		return
	}

	log.Printf("[ROUTING] Starting iterative lookup for Key '%s'", key)

	// 3. Starting iterative lookup for key
	for {
		// Sort the shortlist by distance to the targetId
		sort.Slice(shortList, func(i, j int) bool {
			var IdI, IdJ NodeId
			copy(IdI[:], shortList[i].Id)
			copy(IdJ[:], shortList[j].Id)

			distI := xorDistance(IdI, targetId)
			distJ := xorDistance(IdJ, targetId)

			return isCloser(distI, distJ)
		})

		// find the closest node we haven't queried yet
		var nextNode *proto.Node
		for _, n := range shortList {
			if !queried[string(n.Id)] {
				nextNode = n
				break
			}
		}

		// If all known nodes have been queried, the lookup fails
		if nextNode == nil {
			log.Printf("[RETRIEVE] Lookup exhausted. Key '%s' not found.", key)
			return
		}

		// Mark node as queried
		queried[string(nextNode.Id)] = true
		log.Printf("[ROUTING] Querying node %x at %s", nextNode.Id[:4], nextNode.IpAddress)

		// make the grpc call
		conn, err := grpc.NewClient(nextNode.IpAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("[NETWORK] Dial failed: %v", err)
			continue // Skip to the next node in the shortlist
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)

		grpcClient := proto.NewDHTClient(conn)

		req := &proto.FindValueRequest{
			Sender: c.Self,
			Key:    key,
		}

		resp, err := grpcClient.FindValue(ctx, req)
		conn.Close()
		cancel()

		if err != nil {
			log.Printf("[NETWORK] FindValue RPC to %x failed: %v", nextNode.Id[:4], err)
			continue
		}

		// 4. Process the Response
		if resp.Found {
			log.Printf("[RETRIEVE] Success! Node %x returned value: '%s'", nextNode.Id[:4], string(resp.Value))
			return
		}

		log.Printf("[ROUTING] Node %x did not have the key. It returned %d closer nodes.", nextNode.Id[:4], len(resp.Nodes))

		// Add newly discovered nodes to our routing table and our lookup shortlist
		for _, n := range resp.Nodes {
			c.RoutingTable.AddNode(n)

			if !containsNode(shortList, n) {
				shortList = append(shortList, n)
			}
		}
	}
}
