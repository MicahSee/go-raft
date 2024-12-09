package raft

import (
	"context"
	"fmt"
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
		node.state = "candidate"
		node.votedFor = node.id
		node.votes = 1 // Vote for itself
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
					node.state = "follower"
					node.mu.Unlock()

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
		timeout := time.After(300 * time.Millisecond)

		for {
			select {
			case <-node.heartbeatCh:
				log.Printf("Node %s: Received heartbeat, stopping election.", node.id)
				node.mu.Lock()
				node.state = "follower"
				node.mu.Unlock()
				cancel()
				return

			case <-node.electionCh:
				log.Printf("Node %s: Received election signal with higher term, stopping election.", node.id)
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
				log.Printf("Node %s: Election timeout, retrying.", node.id)
				cancel()
				goto RetryElection
			}
		}

	ElectionResult:
		node.mu.Lock()
		if votes >= majority {
			node.state = "leader"
			node.leaderID = node.id
			node.mu.Unlock()
			log.Printf("Node %s: Won election for term %d with %d votes", node.id, currentTerm, votes)
			go node.sendHeartbeats()
			return
		} else {
			log.Printf("Node %s: Lost election for term %d.", node.id, currentTerm)
			node.state = "follower"
			node.votedFor = ""
		}
		node.mu.Unlock()

	RetryElection:
		time.Sleep(time.Duration(150+rand.Intn(150)) * time.Millisecond) // Random backoff
	}
}

func (node *RaftNode) sendHeartbeats() {
	for {
		node.mu.Lock()
		if node.state != "leader" {
			node.mu.Unlock()
			return
		}
		node.mu.Unlock()

		for _, peer := range node.peers {
			go func(peer string) {
				conn, err := grpc.NewClient(peer, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					return
				}
				defer conn.Close()

				client := pb.NewRaftClient(conn)
				req := &pb.AppendEntriesRequest{
					Term:     node.currentTerm,
					LeaderId: node.id,
				}

				_, err = client.AppendEntries(context.Background(), req)
				if err != nil {
					log.Printf("Node %s: Failed to send heartbeat to %s: %v", node.id, peer, err)
				} else {
					log.Printf("Node %s: Sent heartbeat to %s", node.id, peer)
				}
			}(peer)
		}
		time.Sleep(100 * time.Millisecond) // Heartbeat interval
	}
}

func (node *RaftNode) MonitorHeartbeats() {
	timer := time.NewTimer(HEARTBEAT_TIMEOUT * time.Millisecond) // Timeout duration
	defer timer.Stop()

	for {

		// only check for timeout if the node is a follower
		node.mu.Lock()

		if node.state != "follower" {
			fmt.Println("Node is not a follower")
			node.mu.Unlock()
			time.Sleep(100 * time.Millisecond) // should we sleep here?
			continue
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
			node.mu.Lock()
			node.currentTerm++
			node.mu.Unlock()

			node.StartElection()

			timer.Reset(HEARTBEAT_TIMEOUT * time.Millisecond)
		}
	}
}
