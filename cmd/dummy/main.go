package main

import (
	"context"
	"log"
	"time"

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

func main() {
	factory := func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial("0.0.0.0:4401", grpc.WithInsecure())
		if err != nil {
			log.Println(err)
		}
		return conn, err
	}
	var err error
	pool, err := grpcpool.New(factory, 2, 6, 5*time.Second)
	if err != nil {
		log.Println(err)
	}

	failLeaf(pool, 0)
}
