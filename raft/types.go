package raft

import (
	"fmt"
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

	log          Log
	prevLogIndex int32
	prevLogTerm  int32
	commitIndex  int32
	appliedIndex int32

	stateMachine int32

	clientMap map[string]string

	pb.RaftServer
}

// NewRaftNode initializes a new Raft node
func NewRaftNode(id string, peers []string) *RaftNode {
	return &RaftNode{
		id:          id,
		state:       "candidate",
		currentTerm: 0,
		votedFor:    "",
		peers:       peers,
		heartbeatCh: make(chan struct{}, 1),
		electionCh:  make(chan struct{}, 1),
		log:         NewLog(),
	}
}

type LogEntry struct {
	Term    int32
	Index   int32
	Command string
}

type Log struct {
	entries []LogEntry
	offset  int // Maps global index to slice index
}

func NewLog() Log {
	return Log{
		entries: make([]LogEntry, 0),
		offset:  0,
	}
}

func (l *Log) Append(entry LogEntry) {
	l.entries = append(l.entries, entry)
}

func (l *Log) Get(index int) (*LogEntry, error) {
	if index < l.offset || index >= l.offset+len(l.entries) {
		return nil, fmt.Errorf("index out of range")
	}
	return &l.entries[index-l.offset], nil
}

func (l *Log) Truncate(prefix int) {
	if prefix > l.offset {
		l.entries = l.entries[prefix-l.offset:]
		l.offset = prefix
	}
}
