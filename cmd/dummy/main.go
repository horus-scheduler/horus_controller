package main

import (
	"context"
	"log"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"google.golang.org/grpc"
)

func failLeaf(pool *grpcpool.Pool, leafID uint32) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		log.Println(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusTopologyClient(conn.ClientConn)
	leafInfo := &horus_pb.LeafInfo{Id: leafID, SpineID: 0}

	resp, _ := client.FailLeaf(context.Background(), leafInfo)
	log.Println(resp.Status)
}

func getVCs(pool *grpcpool.Pool) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		log.Println(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusVCClient(conn.ClientConn)
	resp, _ := client.GetVCs(context.Background(), &empty.Empty{})
	log.Println(resp.Vcs)
}

func createTopoPool() *grpcpool.Pool {
	topoFactory := func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial("0.0.0.0:4401", grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	var err error
	topoPool, err := grpcpool.New(topoFactory, 2, 6, 5*time.Second)
	if err != nil {
		log.Println(err)
	}

	return topoPool
}

func createVCPool() *grpcpool.Pool {
	vcFactory := func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial("0.0.0.0:3301", grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	vcPool, err := grpcpool.New(vcFactory, 2, 6, 5*time.Second)
	if err != nil {
		log.Println(err)
	}
	return vcPool
}

func main() {
	// topoPool := createTopoPool()
	// failLeaf(topoPool, 0)

	vcPool := createVCPool()
	getVCs(vcPool)
}
