package raft

import (
	"context"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/MicahSee/go-raft/proto/gen"
)

// Serve starts the gRPC server
func (node *RaftNode) Serve(port string) {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Node %s: Failed to start server: %v", node.id, err)
	}

	node.grpcServer = grpc.NewServer()
	pb.RegisterRaftServer(node.grpcServer, node)

	log.Printf("Node %s: Listening on port %s", node.id, port)
	if err := node.grpcServer.Serve(listener); err != nil {
		log.Fatalf("Node %s: Failed to serve: %v", node.id, err)
	}
}

// StartElection triggers a leader election
func (node *RaftNode) StartElection() {
	for {
		node.mu.Lock()
		if node.state != StateCandidate {
			node.mu.Unlock()
			return
		}

		node.votedFor = node.id
		node.votes = 1 // Vote for itself
		node.currentTerm++
		currentTerm := node.currentTerm
		node.mu.Unlock()

		log.Printf("Node %s: Starting election for term %d", node.id, currentTerm)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var wg sync.WaitGroup
		votesCh := make(chan bool, len(node.peers))

		for _, peer := range node.peers {
			wg.Add(1)
			go func(peer string) {
				defer wg.Done()

				select {
				case <-ctx.Done():
					log.Println("Context canceled, stopping vote request")
					return
				default:
				}

				conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					// log.Printf("Node %s: Could not connect to peer %s: %v", node.id, peer, err)
					votesCh <- false
					return
				}
				defer conn.Close()

				client := pb.NewRaftClient(conn)
				req := &pb.VoteRequest{
					Term:        currentTerm,
					CandidateId: node.id,
				}

				resp, err := client.RequestVote(context.Background(), req)
				if err != nil {
					// log.Printf("Node %s: RequestVote failed to peer %s: %v", node.id, peer, err)
					votesCh <- false
					return
				}

				if resp.Term > currentTerm {
					node.mu.Lock()
					node.currentTerm = resp.Term
					node.mu.Unlock()

					node.transitionState(StateFollower)

					cancel()

					select {
					case node.electionCh <- struct{}{}:
					default: // Non-blocking send
					}

					return
				}

				votesCh <- resp.VoteGranted
			}(peer)
		}

		go func() {
			wg.Wait()
			close(votesCh)
		}()

		majority := len(node.peers)/2 + 1
		votes := 1
		// timeout for contacting all peers
		timeout := time.After(300 * time.Millisecond)

		for {
			select {
			case <-node.heartbeatCh:
				node.transitionState(StateFollower)
				cancel()
				return

			case <-node.electionCh:
				cancel()
				return

			case vote, ok := <-votesCh:
				if !ok {
					goto ElectionResult
				}
				if vote {
					votes++
				}
				if votes >= majority {
					goto ElectionResult
				}

			case <-timeout:
				cancel()
				goto RetryElection
			}
		}

	ElectionResult:
		if votes >= majority {
			node.transitionState(StateLeader)

			node.mu.Lock()
			node.leaderID = node.id
			node.mu.Unlock()
			log.Printf("Node %s: Won election for term %d with %d votes", node.id, currentTerm, votes)
			go node.heartbeatBroadcast()
			return
		} else {
			log.Printf("Node %s: Lost election for term %d.", node.id, currentTerm)

			node.mu.Lock()
			node.votedFor = ""
			node.mu.Unlock()
		}

	RetryElection:
		// random backoff before retrying election
		time.Sleep(time.Duration(MIN_ELECTION_TIMEOUT+rand.Intn(MAX_ELECTION_TIMEOUT-MIN_ELECTION_TIMEOUT)) * time.Millisecond) // Random backoff
	}
}

func (node *RaftNode) MonitorHeartbeats() {
	// start with empty timer
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		// only check for timeout if the node is a follower
		node.mu.Lock()

		if node.state != StateFollower || node.leaderID == "" {
			node.mu.Unlock()
			time.Sleep(100 * time.Millisecond) // should we sleep here?
			continue
		} else if timer == nil {
			timer = time.NewTimer(HEARTBEAT_TIMEOUT * time.Millisecond) // Timeout duration
		}
		node.mu.Unlock()

		select {
		case <-node.heartbeatCh:
			// Received a heartbeat; reset the timer
			if !timer.Stop() {
				<-timer.C // Drain the channel if needed
			}
			timer.Reset(HEARTBEAT_TIMEOUT * time.Millisecond)
			log.Printf("Node %s: Heartbeat received, resetting timeout.", node.id)

		case <-timer.C:
			// Timer expired; restart election
			log.Printf("Node %s: Heartbeat timeout, starting new election.", node.id)

			// increment term
			node.transitionState(StateCandidate)

			// start election
			node.StartElection()

			timer.Reset(HEARTBEAT_TIMEOUT * time.Millisecond)
		}
	}
}

// function to handle role transitions
func (node *RaftNode) transitionState(newState string) {
	node.mu.Lock()
	defer node.mu.Unlock()

	// check if the newState is valid
	if newState != StateFollower && newState != StateCandidate && newState != StateLeader {
		log.Printf("Node %s: Invalid state transition to %s", node.id, newState)
		return
	}

	node.state = newState
}
