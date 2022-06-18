package main

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// Topology-related APIs

func failLeaf(pool *grpcpool.Pool, leafID uint32) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	leafInfo := &horus_pb.LeafInfo{Id: leafID, SpineID: 0}

	resp, _ := client.FailLeaf(context.Background(), leafInfo)
	logrus.Info(resp.Status)
}

func failServer(pool *grpcpool.Pool, serverID uint32) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	serverInfo := &horus_pb.ServerInfo{Id: serverID}

	resp, _ := client.FailServer(context.Background(), serverInfo)
	logrus.Info(resp.Status)
}

func addLeaf(pool *grpcpool.Pool,
	leafID, spineID uint32,
	address, mgmtAddress string,
) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	leafInfo := &horus_pb.LeafInfo{
		Id:          leafID,
		SpineID:     spineID,
		MgmtAddress: mgmtAddress,
		Address:     address,
	}

	resp, _ := client.AddLeaf(context.Background(), leafInfo)
	logrus.Info(resp.Status)
}

func addServer(pool *grpcpool.Pool,
	serverID, portID, workersCount uint32,
	address string,
	leafId uint32,
) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	serverInfo := &horus_pb.ServerInfo{
		Id:           serverID,
		PortId:       portID,
		Address:      address,
		WorkersCount: workersCount,
		LeafID:       leafId,
	}

	resp, _ := client.AddServer(context.Background(), serverInfo)
	logrus.Info(resp.Status)
}

// VC-related APIs
func getVCs(pool *grpcpool.Pool) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	resp, _ := client.GetVCs(context.Background(), &empty.Empty{})
	for _, v := range resp.Vcs {
		logrus.Info(v.Id)
		logrus.Info(v.Spines)
		for _, s := range v.Servers {
			logrus.Info(s.Id)
		}
		logrus.Info()
	}
}

func addVC(pool *grpcpool.Pool,
	vcID uint32,
	spines []uint32,
	servers []uint32,
) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	vcInfo := &horus_pb.VCInfo{
		Id:     vcID,
		Spines: spines,
	}
	for _, sID := range servers {
		vcInfo.Servers = append(vcInfo.Servers, &horus_pb.VCServerInfo{
			Id: sID,
		})
	}

	resp, err := client.AddVC(context.Background(), vcInfo)
	if resp != nil {
		logrus.Info(resp.Status)
	}
	if err != nil {
		logrus.Error(err)
	}
}

func removeVC(pool *grpcpool.Pool,
	vcID uint32,
) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	vcInfo := &horus_pb.VCInfo{
		Id: vcID,
	}

	resp, err := client.RemoveVC(context.Background(), vcInfo)
	if resp != nil {
		logrus.Info(resp.Status)
	}
	if err != nil {
		logrus.Error(err)
	}
}

func createPool() *grpcpool.Pool {
	factory := func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial("0.0.0.0:4001", grpc.WithInsecure())
		if err != nil {
			logrus.Error(err)
		}
		return conn, err
	}
	var err error
	pool, err := grpcpool.New(factory, 2, 6, 5*time.Second)
	if err != nil {
		logrus.Error(err)
	}

	return pool
}

func test_fail_three_servers_then_leaf(pool *grpcpool.Pool) {
	logrus.Info("Failing server 0")
	failServer(pool, 0)
	time.Sleep(time.Second)

	logrus.Info("Failing server 1")
	failServer(pool, 1)
	time.Sleep(time.Second)

	logrus.Info("Failing server 2")
	failServer(pool, 2)
	time.Sleep(time.Second)

	logrus.Info("Failing leaf 0")
	failLeaf(pool, 0)
}

func test_add_two_servers(pool *grpcpool.Pool) {
	// This one should succeed
	addServer(pool, 9, 1, 8, "", 0)
	// This one should fail
	addServer(pool, 10, 1, 8, "", 5)
}

func test_add_leaf_with_servers_then_fail(pool *grpcpool.Pool) {
	addLeaf(pool, 3, 0, "0.0.0.0:6004", "0.0.0.0:7001")
	time.Sleep(5 * time.Second)
	addServer(pool, 9, 1, 8, "", 3)
	time.Sleep(time.Second)
	addServer(pool, 10, 1, 8, "", 3)
	time.Sleep(time.Second)
	failLeaf(pool, 3)
}

func main() {
	pool := createPool()
	// test_fail_three_servers_then_leaf(pool)
	// test_add_two_servers(pool)
	// test_add_leaf_with_servers_then_fail(pool)

	addVC(pool, 2, []uint32{0}, []uint32{2, 3, 4})
	time.Sleep(2 * time.Second)
	removeVC(pool, 2)
	// getVCs(vcPool)
}
