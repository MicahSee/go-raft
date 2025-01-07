package main

import (
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/MicahSee/go-raft/raft"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: %s <node-id> <port> <peer-addresses>\n", os.Args[0])
		os.Exit(1)
	}

	id := os.Args[1]
	port := os.Args[2]
	peers := os.Args[3:]

	node := raft.NewRaftNode(id, peers)

	// Start the gRPC server
	go node.Serve(port)

	// Random delay to simulate election timeout
	time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)

	// Start the election
	node.StartElection()

	fmt.Println("Leader elected. Raft node is running. Press Ctrl+C to stop.")

	// Monitor heartbeats
	go node.MonitorHeartbeats()

	// infinite loop to keep the program running
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Wait()
}
