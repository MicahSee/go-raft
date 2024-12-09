package raft

import (
	"sync"

	"google.golang.org/grpc"

	pb "github.com/MicahSee/go-raft/proto/gen"
)

// RaftNode represents a single Raft node
type RaftNode struct {
	mu          sync.Mutex
	id          string
	currentTerm int32
	leaderID    string
	votedFor    string
	state       string // "follower", "candidate", "leader"
	votes       int32
	peers       []string
	grpcServer  *grpc.Server

	heartbeatCh chan struct{} // Channel to signal heartbeat events
	electionCh  chan struct{} // Channel to signal election events

	pb.RaftServer
}

// NewRaftNode initializes a new Raft node
func NewRaftNode(id string, peers []string) *RaftNode {
	return &RaftNode{
		id:          id,
		state:       "follower",
		currentTerm: 0,
		votedFor:    "",
		peers:       peers,
		heartbeatCh: make(chan struct{}, 1),
		electionCh:  make(chan struct{}, 1),
	}
}
