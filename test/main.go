package main

import (
	"fmt"

	pb "github.com/MicahSee/go-raft/proto"
)

func main() {
	request := &pb.HelloRequest{Name: "World"}
	fmt.Println("Request:", request)
}
