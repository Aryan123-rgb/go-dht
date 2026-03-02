package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/Aryan123-rgb/go-dht/proto"
	"google.golang.org/grpc"
)

func main() {
	// 1. Parse command line flags for port and bootstrapping
	port := flag.String("port", "5001", "Port to listen on")
	bootstrap := flag.String("bootstrap", "", "Address of bootstrap node (e.g., 127.0.0.1:5001)")
	flag.Parse()

	address := fmt.Sprintf("127.0.0.1:%s", *port)

	// 2. Initialize the Node identity
	selfId := GenerateID(address)
	selfNode := &proto.Node{
		Id:        selfId[:],
		IpAddress: address,
	}

	// 3. Initialize the core DHT components
	rt := NewRoutingTable(selfId)
	server := NewDHTServer(selfNode, rt)
	client := NewClient(rt, server, selfNode)

	// 4. Start the gRPC Server in a background goroutine
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	proto.RegisterDHTServer(grpcServer, server)

	go func() {
		log.Printf("[STARTUP] Node %x started on %s", selfId[:4], address)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// 5. Connect to the network if a bootstrap is provided
	if *bootstrap != "" {
		client.Bootstrap(*bootstrap)
	}

	// 6. Start the interactive CLI loop
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("\n--- DHT Node Interactive CLI ---")
	fmt.Println("Commands:")
	fmt.Println("  store <key> <value>  - Store a key-value pair in the network")
	fmt.Println("  get <key>            - Retrieve a value from the network")
	fmt.Println("  nodes                - List all discovered nodes in routing table")
	fmt.Println("  exit                 - Shutdown node")

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		parts := strings.Split(input, " ")

		if len(parts) == 0 || parts[0] == "" {
			continue
		}

		switch parts[0] {
		case "store":
			if len(parts) < 3 {
				fmt.Println("Usage: store <key> <value>")
				continue
			}
			key := parts[1]
			value := strings.Join(parts[2:], " ")
			client.StoreNetwork(key, []byte(value))

		case "get":
			if len(parts) < 2 {
				fmt.Println("Usage: get <key>")
				continue
			}
			client.FindValueNetwork(parts[1])

		case "nodes":
			nodes := rt.GetClosestNode()
			fmt.Printf("Known Nodes (%d):\n", len(nodes))
			for _, n := range nodes {
				fmt.Printf("- %x at %s\n", n.Id[:4], n.IpAddress)
			}

		case "exit":
			grpcServer.Stop()
			os.Exit(0)

		default:
			fmt.Println("Unknown command")
		}
	}
}
