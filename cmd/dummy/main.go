package main

import (
	"context"
	"log"
	"time"

	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"google.golang.org/grpc"
)

func main() {
	var factory grpcpool.Factory
	factory = func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial("0.0.0.0:8800", grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	var err error
	connPool, err := grpcpool.New(factory, 2, 6, 5*time.Second)
	if err != nil {
		log.Println(err)
	}

	conn, err := connPool.Get(context.Background())
	if err != nil {
		log.Println(err)
	}
	defer conn.Close()

	client := horus_pb.NewMdcAppNotifierClient(conn.ClientConn)
	appEvent := &horus_pb.MdcAppEvent{}
	appEvent.TorId = 0
	appEvent.SessionAddress = []byte{0x1a, 0x1b}
	appEvent.HostId = 0
	appEvent.Type = horus_pb.MdcAppEvent_Create
	resp, err := client.StartSession(context.Background(), appEvent)
	log.Println(resp.Status)

	time.Sleep(time.Second)

	// Create a Unix domain socket client
	// Create a Unix domain socket server at the Agent

	appEvent = &horus_pb.MdcAppEvent{}
	appEvent.TorId = 0
	appEvent.SessionAddress = []byte{0x1a, 0x1b}
	appEvent.HostId = 1
	appEvent.Type = horus_pb.MdcAppEvent_Join
	resp, err = client.StartSession(context.Background(), appEvent)
	log.Println(resp.Status)

	// This for experiments *ONLY*
	// Create Timestamp1
	// Wait
	// Recv a pkt from the Agent (via the Unix domain sock) and create Timestamp2
	// ResponseTime = Timestamp2 - Timestamp1

	time.Sleep(time.Second)

	appEvent = &horus_pb.MdcAppEvent{}
	appEvent.TorId = 0
	appEvent.SessionAddress = []byte{0x1a, 0x1b}
	appEvent.HostId = 2
	appEvent.Type = horus_pb.MdcAppEvent_Join
	resp, err = client.StartSession(context.Background(), appEvent)
	log.Println(resp.Status)
}
