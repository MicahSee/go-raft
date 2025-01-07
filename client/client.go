package client

import (
	"context"
	"log"

	pb "github.com/MicahSee/go-raft/proto/gen"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ClientNode struct {
	leaderId   string
	leaderAddr string

	pb.ClientServer
}

func (clientNode *ClientNode) ClientNotification(ctx context.Context, req *pb.ClientRequestCompletion) error {
	log.Printf("Received notification that request %s was completed", req.RequestId)

	return nil
}

func NewClientNode(leaderAddr string) *ClientNode {
	return &ClientNode{leaderId: "leader 1", leaderAddr: leaderAddr}
}

func (clientNode *ClientNode) sendNewRequest(newCmd string) {
	// open new gRPC connection to leader
	conn, err := grpc.NewClient(clientNode.leaderAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Could not connect to peer %s: %v", clientNode.leaderId, err)
		return
	}
	defer conn.Close()

	client := pb.NewRaftClient(conn)
	clientRequest := &pb.ClientRequest{Command: newCmd}

	resp, err := client.ClientGateway(context.Background(), clientRequest)
	if err != nil {
		log.Printf("Error sending client request: %v", err)
		return
	}

	if resp.InProgress {
		log.Printf("Command %s is in progress", newCmd)
		return
	}
}
