package raft

import (
	"context"
	"errors"
	"log"

	pb "github.com/MicahSee/go-raft/proto/gen"
	"github.com/google/uuid"
	"google.golang.org/grpc/peer"
)

// RequestVote handles incoming vote requests (gRPC method)
func (node *RaftNode) RequestVote(ctx context.Context, req *pb.VoteRequest) (*pb.VoteResponse, error) {
	node.mu.Lock()
	defer node.mu.Unlock()

	if req.Term < node.currentTerm {
		return &pb.VoteResponse{Term: node.currentTerm, VoteGranted: false}, nil
	}

	// transition to follower if a vote is requested with a higher term than the current term
	// if req.Term > node.currentTerm {
	// 	node.currentTerm = req.Term
	// 	node.votedFor = ""
	// 	node.state = "follower"
	// }

	if node.votedFor == "" || node.votedFor == req.CandidateId {
		node.votedFor = req.CandidateId
		log.Printf("Node %s: Voted for %s in term %d", node.id, req.CandidateId, req.Term)
		return &pb.VoteResponse{Term: node.currentTerm, VoteGranted: true}, nil
	}

	return &pb.VoteResponse{Term: node.currentTerm, VoteGranted: false}, nil
}

// func (node *RaftNode) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
// 	node.mu.Lock()
// 	defer node.mu.Unlock()

// 	// if there are no entries it's a heartbeat
// 	if len(req.Entries) > 0 {
// 		// if the leader's term is behind the follower's term, reject the request
// 		// if req.Term < node.currentTerm {
// 		// 	return &pb.AppendEntriesResponse{
// 		// 		Term:    node.currentTerm,
// 		// 		Success: false,
// 		// 	}, nil
// 		// }

// 		// // if the leader's term is ahead of the follower's term, update the follower's term
// 		// if req.Term > node.currentTerm {
// 		// 	node.currentTerm = req.Term
// 		// 	node.state = "follower"
// 		// 	node.leaderID = req.LeaderId
// 		// }

// 		// if the follower's log is shorter than the leader's log, reject the request
// 		// if len(node.log.entries) < len(req.Entries) {
// 		// 	return &pb.AppendEntriesResponse{
// 		// 		Term:    node.currentTerm,
// 		// 		Success: false,
// 		// 	}, nil
// 		// }

// 		// if the follower's log is longer than the leader's log, truncate the follower's log
// 		// if len(node.log.entries) > len(req.Entries) {
// 		// 	node.log.Truncate(len(req.Entries))
// 		// }

// 		// if the leader's prevLogIndex doesn't match the follower's prevLogIndex, reject the request
// 		// if len(node.log.entries) > 0 && req.PrevLogIndex >= 0 && req.PrevLogIndex < int32(len(node.log.entries)) {
// 		// 	if node.log.entries[req.PrevLogIndex].Term != req.PrevLogTerm {
// 		// 		return &pb.AppendEntriesResponse{
// 		// 			Term:    node.currentTerm,
// 		// 			Success: false,
// 		// 		}, nil
// 		// 	}
// 		// }
// 		go node.twoPhaseCommit(req.Entries)
// 	}

// 	if req.Term >= node.currentTerm {
// 		node.currentTerm = req.Term
// 		node.state = "follower"
// 		node.leaderID = req.LeaderId

// 		fmt.Printf("Node %s: Received heartbeat from %s\n", node.id, req.LeaderId)

// 		// Notify the node that a leader is active
// 		select {
// 		case node.heartbeatCh <- struct{}{}:
// 		default: // Non-blocking send
// 		}
// 	}

// 	return &pb.AppendEntriesResponse{
// 		Term:    node.currentTerm,
// 		Success: true,
// 	}, nil
// }

func (node *RaftNode) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	if len(req.Entries) != 0 {
		log.Printf("Received new command from leader: %s", req.Entries[0].Command)
		return &pb.AppendEntriesResponse{Success: true}, nil
	} else {
		node.mu.Lock()
		defer node.mu.Unlock()
		// differentiate between heartbeat and commit confirmation
		if req.LeaderCommit > node.commitIndex {
			node.commitIndex = req.LeaderCommit
			go node.commitLogEntry(req.LeaderCommit)
		}
		return &pb.AppendEntriesResponse{Success: true}, nil
	}
}

func newNotLeaderError() error {
	return errors.New("error: you have made a leader-only request to a non-leader node")
}

func (node *RaftNode) ClientGateway(ctx context.Context, req *pb.ClientRequest) (*pb.ClientRequestAck, error) {
	node.mu.Lock()
	defer node.mu.Unlock()

	log.Printf("Received client request: %s", req.Command)

	// Eventually, if the node is not the leader, forward the request to the leader
	// For now, just return an error
	if node.state != "leader" {
		return &pb.ClientRequestAck{InProgress: false}, newNotLeaderError()
	}

	// Append the new command to the leader's log
	entry := LogEntry{
		Term:    node.currentTerm,
		Command: req.Command,
		Index:   int32(len(node.log.entries)),
	}
	node.log.Append(entry)

	// Broadcast the new entry to all peers
	go node.appendLogBroadcast(&entry)

	// Notify the node that there are new entries to replicate
	p, _ := peer.FromContext(ctx)
	uuid := uuid.New()
	node.clientMap[uuid.String()] = p.Addr.String()

	return &pb.ClientRequestAck{InProgress: true}, nil
}
