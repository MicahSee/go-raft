package raft

import (
	"context"
	"fmt"
	"log"

	pb "github.com/MicahSee/go-raft/proto/gen"
)

// RequestVote handles incoming vote requests (gRPC method)
func (node *RaftNode) RequestVote(ctx context.Context, req *pb.VoteRequest) (*pb.VoteResponse, error) {
	node.mu.Lock()
	defer node.mu.Unlock()

	if req.Term < node.currentTerm {
		return &pb.VoteResponse{Term: node.currentTerm, VoteGranted: false}, nil
	}

	// transition to follower if a vote is requested with a higher term than the current term
	if req.Term > node.currentTerm {
		node.currentTerm = req.Term
		node.votedFor = ""
		node.state = "follower"
	}

	if node.votedFor == "" || node.votedFor == req.CandidateId {
		node.votedFor = req.CandidateId
		log.Printf("Node %s: Voted for %s in term %d", node.id, req.CandidateId, req.Term)
		return &pb.VoteResponse{Term: node.currentTerm, VoteGranted: true}, nil
	}

	return &pb.VoteResponse{Term: node.currentTerm, VoteGranted: false}, nil
}

func (node *RaftNode) AppendEntries(ctx context.Context, req *pb.AppendEntriesRequest) (*pb.AppendEntriesResponse, error) {
	node.mu.Lock()
	defer node.mu.Unlock()

	if req.Term >= node.currentTerm {
		node.currentTerm = req.Term
		node.state = "follower"
		node.leaderID = req.LeaderId

		fmt.Printf("Node %s: Received heartbeat from %s\n", node.id, req.LeaderId)

		// Notify the node that a leader is active
		select {
		case node.heartbeatCh <- struct{}{}:
		default: // Non-blocking send
		}
	}

	select {
	case <-ctx.Done():
		log.Printf("Node %s: Request canceled by caller", node.id)
		return nil, ctx.Err() // Respond with the context error
	default:
	}

	return &pb.AppendEntriesResponse{
		Term:    node.currentTerm,
		Success: true,
	}, nil
}
