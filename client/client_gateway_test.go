package client

import (
	"strconv"
	"testing"
	"time"

	"github.com/MicahSee/go-raft/raft"
)

func TestClientGateway(t *testing.T) {
	// Create follower nodes
	peers := []string{"127.0.0.1:5001", "127.0.0.1:5002", "127.0.0.1:5003"}

	for i, _ := range peers {
		// Create a new slice to hold a copy of peers excluding peers[i]
		peerCopy := make([]string, 0, len(peers)-1)

		// Append elements from peers[:i] and peers[i+1:] manually
		peerCopy = append(peerCopy, peers[:i]...)
		peerCopy = append(peerCopy, peers[i+1:]...)

		port := "500" + strconv.Itoa(i+1)

		ln := raft.NewRaftNode("follower"+strconv.Itoa(i), peerCopy)
		ln.TransitionState(raft.StateFollower)
		go ln.Serve(port)
	}

	// Create leader node
	leaderPort := "5009"
	leaderAddr := "localhost:" + leaderPort

	ln := raft.NewRaftNode("leader0", peers)
	ln.TransitionState(raft.StateLeader)
	go ln.Serve(leaderPort)
	go ln.InitiateHeartbeatProcess()

	// Create client node
	cn := NewClientNode(leaderAddr)

	// Send the command to the leader
	command := "set state=10"
	cn.sendNewRequest(command)

	// Wait for 2 second to allow the leader to broadcast the command to followers
	time.Sleep(2 * time.Second)
}
