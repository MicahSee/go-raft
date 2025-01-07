package raft

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/MicahSee/go-raft/proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// commit the log entry, update the commit index, and notify peers
func (node *RaftNode) commitLogEntry(logIndex int32) {
	node.mu.Lock()
	defer node.mu.Unlock()

	// Find the log entry
	entry, err := node.log.Get(int(logIndex))
	if err != nil {
		log.Printf("Failed to commit log entry %d: %v", logIndex, err)
		return
	}

	// Decode the command and apply it to the state machine
	substr := strings.Split(entry.Command, "=")[1]
	newVal, _ := strconv.Atoi(substr)

	// Update the state machine
	node.stateMachine = int32(newVal)
	log.Printf("Node %s: Committed log entry %d with command %s", node.id, logIndex, entry.Command)
	log.Printf("Node %s: State machine updated to %d", node.id, node.stateMachine)

	// Update the commit index
	node.commitIndex = logIndex

	// Peers are already notified of the updated commit index via heartbeats
}

// generic function for sending append entries request to peers
// it should be able to handle both heartbeat and log entry append requests
func (node *RaftNode) sendAppendEntriesRequest(peer string, req *pb.AppendEntriesRequest, reqType string) (success bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// Establish a connection to the peer
	conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Could not connect to peer %s: %v", peer, err)
		return false, err
	}
	defer conn.Close()

	client := pb.NewRaftClient(conn)
	resp, err := client.AppendEntries(ctx, req)
	if err != nil {
		log.Printf("AppendEntries %s request failed to peer %s: %v", reqType, peer, err)
		return false, err
	}

	// Handle response
	if resp.Success {
		log.Printf("AppendEntries %s request succeeded to peer %s", reqType, peer)
		return true, nil
	} else {
		log.Printf("AppendEntries %s request rejected by peer %s", reqType, peer)
		if resp.Term > node.currentTerm {
			node.mu.Lock()
			node.currentTerm = resp.Term
			node.state = StateFollower // Step down to follower if a higher term is observed
			node.mu.Unlock()
		}
		return false, nil
	}
}

// Broadcast log entries to peers
func (node *RaftNode) appendLogBroadcast(entry *LogEntry) {
	var wg sync.WaitGroup
	var successCount int32
	majority := len(node.peers)/2 + 1

	// Lock the node to get the previous log index and term
	node.mu.Lock()
	prevLogIndex := node.prevLogIndex
	prevLogTerm := node.prevLogTerm
	commitIndex := node.commitIndex
	term := node.currentTerm
	leaderID := node.id
	node.mu.Unlock()

	// Create the AppendEntriesRequest for the log entry
	logEntryRequest := &pb.AppendEntriesRequest{
		Term:         term,
		LeaderId:     leaderID,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      []*pb.LogEntry{{Term: entry.Term, Index: entry.Index, Command: entry.Command}},
		LeaderCommit: commitIndex,
	}

	// Send requests to all peers
	for _, peer := range node.peers {
		wg.Add(1)
		go func(peer string) {
			defer wg.Done()
			success, _ := node.sendAppendEntriesRequest(peer, logEntryRequest, "log")
			if success {
				atomic.AddInt32(&successCount, 1)
			}
		}(peer)
	}

	// Wait for all goroutines to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Check if a majority of peers accepted the entry
	select {
	case <-done:
		if successCount >= int32(majority) {
			log.Println("Majority achieved, committing entry")
			node.commitLogEntry(entry.Index)

			// notify client of successful commit
		} else {
			log.Println("Majority not achieved, aborting")
		}
	case <-time.After(300 * time.Millisecond):
		log.Println("Timeout reached while broadcasting append entries")
	}
}

// function to broadcast heartbeat messages to peers
func (node *RaftNode) heartbeatBroadcast() {
	// Lock the node to get the current term and leader ID
	for {
		node.mu.Lock()
		term := node.currentTerm
		leaderID := node.id
		commitIndex := node.commitIndex
		node.mu.Unlock()

		// Create the AppendEntriesRequest for the heartbeat
		heartbeatRequest := &pb.AppendEntriesRequest{
			Term:         term,
			LeaderId:     leaderID,
			PrevLogIndex: commitIndex,
			PrevLogTerm:  term,
			Entries:      []*pb.LogEntry{},
			LeaderCommit: commitIndex,
		}

		// Send requests to all peers
		for _, peer := range node.peers {
			go func(peer string) {
				success, _ := node.sendAppendEntriesRequest(peer, heartbeatRequest, "heartbeat")
				if !success {
					log.Printf("Failed to send heartbeat to peer %s", peer)
				}
			}(peer)
		}

		// Wait for the next heartbeat interval
		time.Sleep(HEARTBEAT_TIMEOUT * time.Millisecond)
	}
}
