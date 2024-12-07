package main

import (
	"context"
	"log"
	"net"

	"github.com/MicahSee/go-raft/proto/gen"
	"google.golang.org/grpc"
)

type greeterServer struct {
	gen.UnimplementedGreeterServer
}

func (s *greeterServer) SayHello(ctx context.Context, req *gen.HelloRequest) (*gen.HelloReply, error) {
	log.Printf("Received request from: %s", req.Name)
	return &gen.HelloReply{Message: "Hello " + req.Name}, nil
}

func main() {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	gen.RegisterGreeterServer(grpcServer, &greeterServer{})

	log.Println("Server is running on port 50051...")
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
