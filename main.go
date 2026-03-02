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

// ANSI Color Codes for terminal formatting
const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	Bold   = "\033[1m"
)

func printBanner(port string) {
	fmt.Print(Cyan + Bold)
	fmt.Println(`
    ____  __  __  ______   _   __          __     
   / __ \/ / / / /_  __/  / | / /___  ____/ /___ 
  / / / / /_/ /   / /    /  |/ / __ \/ __  / __ \
 / /_/ / __  /   / /    / /|  / /_/ / /_/ /  __/
/_____/_/ /_/   /_/    /_/ |_/\____/\__,_/\___/ 
                                                `)
	fmt.Printf("      Kademlia DHT Node Active on Port %s\n", port)
	fmt.Println(Reset)
}

func printHelp() {
	fmt.Println(Bold + "\nCOMMANDS AVAILABLE:" + Reset)
	fmt.Printf("  %sstore%s <key> <value>  %s::%s Store a key-value pair in the network\n", Green, Reset, Yellow, Reset)
	fmt.Printf("  %sget%s <key>            %s::%s Retrieve a value from the network\n", Cyan, Reset, Yellow, Reset)
	fmt.Printf("  %snodes%s                %s::%s List all discovered nodes in routing table\n", Purple, Reset, Yellow, Reset)
	fmt.Printf("  %sexit%s                 %s::%s Gracefully shutdown the node\n", Red, Reset, Yellow, Reset)
}

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
		// Keep server logs default so they stand out from user input
		log.Printf("[STARTUP] Node %x started on %s", selfId[:4], address)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// 5. Connect to the network if a bootstrap is provided
	if *bootstrap != "" {
		client.Bootstrap(*bootstrap)
	}

	// Wait a brief moment for startup logs to print before showing the UI
	printBanner(*port)
	printHelp()

	// 6. Start the interactive CLI loop
	scanner := bufio.NewScanner(os.Stdin)

	for {
		// Custom colored prompt
		fmt.Printf("\n%s[Node %x]%s ➜ ", Green+Bold, selfId[:4], Reset)
		
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
				fmt.Printf("%sError: Usage is `store <key> <value>`%s\n", Red, Reset)
				continue
			}
			key := parts[1]
			value := strings.Join(parts[2:], " ")
			client.StoreNetwork(key, []byte(value))

		case "get":
			if len(parts) < 2 {
				fmt.Printf("%sError: Usage is `get <key>`%s\n", Red, Reset)
				continue
			}
			client.FindValueNetwork(parts[1])

		case "nodes":
			nodes := rt.GetClosestNode()
			fmt.Printf("\n%s=== Routing Table: Known Nodes (%d) ===%s\n", Blue+Bold, len(nodes), Reset)
			if len(nodes) == 0 {
				fmt.Printf("  %s(Empty)%s\n", Yellow, Reset)
			} else {
				for i, n := range nodes {
					fmt.Printf("  %d. %s[%x]%s at %s\n", i+1, Purple, n.Id[:4], Reset, n.IpAddress)
				}
			}
			fmt.Println(Blue + strings.Repeat("=", 39) + Reset)

		case "exit":
			fmt.Printf("%sShutting down... Goodbye!%s\n", Yellow, Reset)
			grpcServer.Stop()
			os.Exit(0)

		case "help":
			printHelp()

		default:
			fmt.Printf("%sUnknown command '%s'. Type 'help' to see available commands.%s\n", Red, parts[0], Reset)
		}
	}
}