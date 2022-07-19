package main

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// Topology-related APIs

func getTopology(pool *grpcpool.Pool) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	topo, _ := client.GetTopology(context.Background(), &empty.Empty{})
	for _, spineInfo := range topo.Spines {

		logrus.Infof("- Spine: %d", spineInfo.Id)
		for _, leafInfo := range spineInfo.Leaves {
			leafFirstWorkerID := 0
			leafLastWorkerID := 0
			var lines []string
			for _, serverInfo := range leafInfo.Servers {
				if leafLastWorkerID > 0 {
					leafLastWorkerID += 1
				}
				serverFWID := leafLastWorkerID
				serverLWID := leafLastWorkerID
				if serverInfo.WorkersCount > 0 {
					serverLWID += int(serverInfo.WorkersCount) - 1
					leafLastWorkerID = serverLWID
				}
				serverLine := fmt.Sprintf("--- Server: %d, First WID: %d, last WID: %d",
					serverInfo.Id, serverFWID, serverLWID)
				lines = append(lines, serverLine)
			}

			logrus.Infof("-- Leaf: %d, Index: %d, First WID: %d, Last WID: %d",
				leafInfo.Id, leafInfo.Index, leafFirstWorkerID, leafLastWorkerID)
			for _, line := range lines {
				logrus.Info(line)
			}
		}
	}
}

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

// Parham: Modified interface to include portID for leaf, also modified referecnes to addLeaf.
func addLeaf(pool *grpcpool.Pool,
	leafID, portID, leafPipeID, spineID uint32,
	address, mgmtAddress string,
) {
	conn, err := pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	// Parham: Added attribute setting portID below
	leafInfo := &horus_pb.LeafInfo{
		Id:          leafID,
		PipeID:      leafPipeID,
		SpineID:     spineID,
		MgmtAddress: mgmtAddress,
		Address:     address,
		PortId:      portID,
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
	time.Sleep(5 * time.Second)
	// Parham: modified call to addLeaf dummy port for leaf (44)
	addLeaf(pool, 0, 44, 65535, 0, "0.0.0.0:6001", "0.0.0.0:7001")
	time.Sleep(5 * time.Second)
	// This one should succeed
	addServer(pool, 9, 1, 8, "99:99:99:99:99:99", 0)
	// This one should fail
	addServer(pool, 10, 1, 8, "aa:aa:aa:aa:aa:aa", 5)
}

func test_add_leaf_with_servers_then_fail(pool *grpcpool.Pool) {
	// Parham: modified call to addLeaf dummy port for leaf (44)
	addLeaf(pool, 3, 44, 65535, 0, "0.0.0.0:6004", "0.0.0.0:7001")
	time.Sleep(5 * time.Second)
	addServer(pool, 10, 1, 8, "aa:aa:aa:aa:aa:aa", 3)
	time.Sleep(time.Second)
	addServer(pool, 11, 1, 8, "bb:bb:bb:bb:bb:bb", 3)
	time.Sleep(time.Second)
	failLeaf(pool, 3)
}

func test_leaf_index(pool *grpcpool.Pool) {
	failLeaf(pool, 1)
	time.Sleep(5 * time.Second)
	// Parham: modified call to addLeaf dummy port for leaf (44)
	addLeaf(pool, 3, 44, 65535, 0, "0.0.0.0:6004", "0.0.0.0:7001")
	time.Sleep(5 * time.Second)
	addServer(pool, 10, 1, 8, "aa:aa:aa:aa:aa:aa", 3)
	time.Sleep(time.Second)
	addServer(pool, 11, 1, 8, "bb:bb:bb:bb:bb:bb", 3)
}

func main() {
	pool := createPool()
	getTopology(pool)
	test_leaf_index(pool)
	// test_fail_three_servers_then_leaf(pool)
	// test_add_two_servers(pool)
	// test_add_leaf_with_servers_then_fail(pool)

	// time.Sleep(5 * time.Second)
	// addVC(pool, 2, []uint32{0}, []uint32{3, 4, 6})
	// addVC(pool, 2, []uint32{0}, []uint32{9})
	// time.Sleep(2 * time.Second)
	// removeVC(pool, 2)
	// getVCs(pool)
}
