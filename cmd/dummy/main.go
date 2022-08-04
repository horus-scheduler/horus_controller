package main

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	grpcpool "github.com/processout/grpc-go-pool"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// Topology-related APIs

type HorusClient struct {
	pool      *grpcpool.Pool
	cCtrlAddr string
	asicStr   string
	asic      *model.Asic
	portMap   *model.PortMap
}

func NewHorusClient(address, asicStr string) *HorusClient {
	client := &HorusClient{cCtrlAddr: address, asicStr: asicStr}
	client.initPool()
	client.getPorts()
	return client
}

func (c *HorusClient) initPool() {
	factory := func() (*grpc.ClientConn, error) {
		conn, err := grpc.Dial(c.cCtrlAddr, grpc.WithInsecure())
		if err != nil {
			logrus.Fatal(err)
		}
		return conn, err
	}
	var err error
	c.pool, err = grpcpool.New(factory, 2, 6, 5*time.Second)
	if err != nil {
		logrus.Fatal(err)
	}
}

func (c *HorusClient) getPorts() {
	conn, err := c.pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()
	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	topo, _ := client.GetTopology(context.Background(), &empty.Empty{})

	ar := model.NewAsicRegistryFromInfo(topo.Asics, topo.PortConfig)
	c.asic, _ = ar.AsicMap.Load(c.asicStr)
	c.portMap = c.asic.PortRegistry.PortMap
}

func (c *HorusClient) GetPortInfoBySpecID(specStr string) *horus_pb.PortInfo {
	port, _ := c.portMap.Load(specStr)
	return port.ToInfo()
}

func (c *HorusClient) GetPortInfoByCageLane(cage uint64, lane uint64) *horus_pb.PortInfo {
	specStr := fmt.Sprintf("%d/%d", cage, lane)
	return c.GetPortInfoBySpecID(specStr)
}

func (c *HorusClient) GetTopology() {
	conn, err := c.pool.Get(context.Background())
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
				serverLine := fmt.Sprintf("--- Server: %d, First WID: %d, last WID: %d, Port: %s, DEVPORT: %d",
					serverInfo.Id,
					serverFWID,
					serverLWID,
					serverInfo.Port.ID,
					serverInfo.Port.DevPort,
				)
				lines = append(lines, serverLine)
			}

			logrus.Infof("-- Leaf: %d, Index: %d, First WID: %d, Last WID: %d, DS Port: %s, DS DEVPORT: %d, US Port: %s, US DEVPORT: %d",
				leafInfo.Id, leafInfo.Index, leafFirstWorkerID, leafLastWorkerID,
				leafInfo.DsPort.ID, leafInfo.DsPort.DevPort,
				leafInfo.UsPort.ID, leafInfo.UsPort.DevPort,
			)
			for _, line := range lines {
				logrus.Info(line)
			}
		}
	}
}

func (c *HorusClient) FailLeaf(leafID uint32) {
	conn, err := c.pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	leafInfo := &horus_pb.LeafInfo{Id: leafID, SpineID: 0}

	resp, _ := client.FailLeaf(context.Background(), leafInfo)
	logrus.Info(resp.Status)
}

func (c *HorusClient) FailServer(serverID uint32) {
	conn, err := c.pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	serverInfo := &horus_pb.ServerInfo{Id: serverID}

	resp, _ := client.FailServer(context.Background(), serverInfo)
	logrus.Info(resp.Status)
}

func (c *HorusClient) AddLeaf(leafID, spineID uint32,
	dsPortStr, usPortStr, address, mgmtAddress string,
) {
	conn, err := c.pool.Get(context.Background())
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
		Asic:        c.asic.ToInfo(),
		DsPort:      c.GetPortInfoBySpecID(dsPortStr),
		UsPort:      c.GetPortInfoBySpecID(usPortStr),
	}

	resp, _ := client.AddLeaf(context.Background(), leafInfo)
	logrus.Info(resp.Status)
}

func (c *HorusClient) AddServer(serverID, workersCount uint32,
	portSpec, address string,
	leafId uint32,
) {
	conn, err := c.pool.Get(context.Background())
	if err != nil {
		logrus.Error(err)
	}
	defer conn.Close()

	client := horus_pb.NewHorusServiceClient(conn.ClientConn)
	serverInfo := &horus_pb.ServerInfo{
		Id:           serverID,
		Port:         c.GetPortInfoBySpecID(portSpec),
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

/*
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
*/

func test_leaf_index(client *HorusClient) {
	client.FailLeaf(1)
	// time.Sleep(5 * time.Second)
	// client.AddLeaf(4, 100, "28/0", "22/0", "0.0.0.0:6005", "0.0.0.0:7001")
	// time.Sleep(25 * time.Second)
	// client.AddServer(10, 8, "9/1", "aa:aa:aa:aa:aa:aa", 4)
	// time.Sleep(time.Second)
	// client.AddServer(11, 8, "9/2", "bb:bb:bb:bb:bb:bb", 4)
}

func main() {
	client := NewHorusClient("0.0.0.0:4001", "tofino")
	client.GetTopology()
	time.Sleep(10 * time.Second)
	test_leaf_index(client)

	// test_leaf_index(pool)
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
