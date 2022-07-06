package net

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
	"google.golang.org/grpc/peer"
)

type centralSrvServer struct {
	horus_pb.UnimplementedHorusServiceServer

	failedLeaves  chan *LeafFailedMessage
	failedServers chan *ServerFailedMessage
	newLeaves     chan *LeafAddedMessage
	newServers    chan *ServerAddedMessage

	newVCs chan *VCUpdatedMessage

	topology *model.Topology
	vcm      *core.VCManager
}

func NewCentralSrvServer(topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage,
	failedServers chan *ServerFailedMessage,
	newLeaves chan *LeafAddedMessage,
	newServers chan *ServerAddedMessage,
	newVCs chan *VCUpdatedMessage,
) *centralSrvServer {
	return &centralSrvServer{
		topology:      topology,
		vcm:           vcm,
		failedLeaves:  failedLeaves,
		failedServers: failedServers,
		newLeaves:     newLeaves,
		newServers:    newServers,
		newVCs:        newVCs,
	}
}

func (s *centralSrvServer) GetTopology(context.Context, *empty.Empty) (*horus_pb.TopoInfo, error) {
	logrus.Debugf("[CentralServer] GetTopology() is called")
	topoInfo := s.topology.EncodeToTopoInfo()
	return topoInfo, nil
}

func (s *centralSrvServer) GetTopologyAtLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.TopoInfo, error) {
	logrus.Debugf("[CentralServer] GetTopologyAtLeaf(%d) is called", leafInfo.Id)
	topoInfo := s.topology.EncodeToTopoInfoAtLeaf(leafInfo)
	if topoInfo == nil {
		return nil, fmt.Errorf("couldn't return TopoInfo for leaf %d", leafInfo.Id)
	}
	return topoInfo, nil
}

func (s *centralSrvServer) AddLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[CentralServer] Adding a leaf %d to spine %d", leafInfo.Id, leafInfo.SpineID)
	leaf, err := s.topology.AddLeafToSpine(leafInfo)
	if err != nil {
		logrus.Error(err)
		return &horus_pb.HorusResponse{Status: "FAILED"}, nil
	}
	if leaf.Parent != nil {
		spine := leaf.Parent
		s.newLeaves <- NewLeafAddedMessage(leafInfo, spine)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}

	logrus.Error("[CentralServer] Adding leaf %d failed because it doesn't belong to a spine", leaf.ID)
	return &horus_pb.HorusResponse{Status: "FAILED"}, nil
}

func (s *centralSrvServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("[CentralServer] Failing a leaf %d", leafID)
	leaf := s.topology.GetNode(leafID, model.NodeType_Leaf)
	if leaf == nil {
		logrus.Debugf("[CentralServer] Leaf %d doesn't exist", leafID)
		return &horus_pb.HorusResponse{Status: "FAILD"}, nil
	}

	spinesMap := model.NewNodeMap()
	// 1. Get Spines belonging to all this leaf's VCs
	vcs, ok := s.vcm.GetVCsOfLeaf(leafID)
	if ok {
		for _, vc := range vcs {
			for spineId, spine := range vc.Spines.Internal() {
				spinesMap.Store(spineId, spine)
			}
		}
	}

	// 2. Get the Spine that this leaf belong to (topology-wise)
	// leaf cannot be nil here.
	if leaf.Parent != nil {
		spinesMap.Store(leaf.Parent.ID, leaf.Parent)
	}

	// 3. Now, append all the unique spines to the `spines` slice
	spines := make([]*model.Node, 0)
	for _, spine := range spinesMap.Internal() {
		spines = append(spines, spine)
	}

	// 4. Detach and remove the leaf
	detached := s.vcm.DetachLeaf(leafID)
	leafIdx := s.topology.RemoveLeaf(leafID)
	if leaf != nil && leafIdx >= 0 && detached {
		s.failedLeaves <- NewLeafFailedMessage(leafInfo, spines)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "FAILD"}, nil
}

func (s *centralSrvServer) AddServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[CentralServer] Adding a server %d", serverInfo.Id)
	server, err := s.topology.AddServerToLeaf(serverInfo, uint16(serverInfo.LeafID))
	if err != nil {
		logrus.Error(err)
		return &horus_pb.HorusResponse{Status: "FAILED"}, nil
	}
	if server.Parent != nil && server.Parent.Parent != nil {
		spine := server.Parent.Parent
		s.newServers <- NewServerAddedMessage(serverInfo, spine)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}

	if server.Parent == nil {
		logrus.Error("[CentralServer] Adding server %d failed because it doesn't belong to a leaf", server.ID)
	} else {
		if server.Parent.Parent == nil {
			logrus.Error("[CentralServer] Adding server %d failed because it doesn't belong to a spine", server.ID)
		}
	}
	return &horus_pb.HorusResponse{Status: "FAILED"}, nil
}

func (s *centralSrvServer) FailServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	serverID := uint16(serverInfo.Id)
	logrus.Debugf("[CentralServer] Failing a server %d", serverID)
	server := s.topology.GetNode(serverID, model.NodeType_Server)
	if server == nil {
		logrus.Debugf("[CentralServer] Server %d doesn't exist", serverID)
		return &horus_pb.HorusResponse{Status: "FAILD"}, nil
	}

	// Spines to be updated
	spinesMap := model.NewNodeMap()
	vcs, ok := s.vcm.GetVCsOfServer(serverID)
	if ok {
		for _, vc := range vcs {
			for spineId, spine := range vc.Spines.Internal() {
				spinesMap.Store(spineId, spine)
			}
		}
	}

	if server.Parent != nil && server.Parent.Parent != nil {
		spinesMap.Store(server.Parent.Parent.ID, server.Parent.Parent)
	}

	spines := make([]*model.Node, 0)
	for _, spine := range spinesMap.Internal() {
		spines = append(spines, spine)
	}
	logrus.Debugf("[CentralServer] Spines count: %d", len(spines))

	detached := s.vcm.DetachServer(serverID)
	removed, _ := s.topology.RemoveServer(serverID)
	logrus.Debugf("[CentralServer] Server %d detached? %t, removed? %t", serverID, detached, removed)
	if removed && detached {
		s.failedServers <- NewServerFailedMessage(serverInfo, spines)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *centralSrvServer) GetVCs(ctx context.Context, e *empty.Empty) (*horus_pb.VCsResponse, error) {
	p, _ := peer.FromContext(ctx)
	logrus.Debugf("[CentralServer] GetVCs() called by %s", p.Addr.String())
	vcs := s.vcm.GetVCs()
	rsp := &horus_pb.VCsResponse{}
	for _, vc := range vcs {
		vcInfo := &horus_pb.VCInfo{}
		vcInfo.Id = uint32(vc.ClusterID)
		for _, server := range vc.Servers.Internal() {
			serverInfo := &horus_pb.VCServerInfo{}
			serverInfo.Id = uint32(server.ID)
			vcInfo.Servers = append(vcInfo.Servers, serverInfo)
		}
		for _, spine := range vc.Spines.Internal() {
			vcInfo.Spines = append(vcInfo.Spines, uint32(spine.ID))
		}
		rsp.Vcs = append(rsp.Vcs, vcInfo)
	}
	return rsp, nil
}

func (s *centralSrvServer) GetVCsOfLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.VCsResponse, error) {
	p, _ := peer.FromContext(ctx)
	logrus.Debugf("[CentralServer] GetVCsOfLeaf(%d) called by %s", leafInfo.Id, p.Addr.String())
	vcs, found := s.vcm.GetVCsOfLeaf(uint16(leafInfo.Id))
	leaf := s.topology.GetNode(uint16(leafInfo.Id), model.NodeType_Leaf)
	if !found || leaf == nil {
		return nil, fmt.Errorf("virtual clusters for leaf %d don't exist", leafInfo.Id)
	}

	rsp := &horus_pb.VCsResponse{}
	for _, vc := range vcs {
		vcInfo := &horus_pb.VCInfo{}
		vcInfo.Id = uint32(vc.ClusterID)
		for _, server := range vc.Servers.Internal() {
			if server.Parent != nil && server.Parent.ID == leaf.ID {
				serverInfo := &horus_pb.VCServerInfo{}
				serverInfo.Id = uint32(server.ID)
				vcInfo.Servers = append(vcInfo.Servers, serverInfo)
			}
		}
		for _, spine := range vc.Spines.Internal() {
			vcInfo.Spines = append(vcInfo.Spines, uint32(spine.ID))
		}
		rsp.Vcs = append(rsp.Vcs, vcInfo)
	}
	return rsp, nil
}

func (s *centralSrvServer) AddVC(ctx context.Context, vcInfo *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[CentralServer] Adding VC: %d", vcInfo.Id)
	vc, err := model.NewVC(vcInfo, s.topology)
	if err != nil {
		logrus.Warnf("[CentralServer] Adding VC: %d failed: %s", vcInfo.Id, err.Error())
		return nil, err
	}
	if ok, err := s.vcm.AddVC(vc); !ok {
		return nil, err
	}

	var spines []*model.Node
	for _, spine := range vc.Spines.Internal() {
		spines = append(spines, spine)
	}

	s.newVCs <- NewVCUpdatedMessage(vcInfo, VCUpdateAdd, spines)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *centralSrvServer) RemoveVC(ctx context.Context, vcInfo *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	clusterID := uint16(vcInfo.Id)
	logrus.Debugf("[CentralServer] Removing VC: %d", clusterID)

	vc := s.vcm.GetVC(clusterID)
	if vc == nil {
		logrus.Warnf("[CentralServer] Removing VC: %d failed: doesn't exist", clusterID)
		return nil, fmt.Errorf("VC %d doesn't exist", clusterID)
	}
	var spines []*model.Node
	for _, spine := range vc.Spines.Internal() {
		spines = append(spines, spine)
	}

	s.vcm.RemoveVC(vc)
	s.newVCs <- NewVCUpdatedMessage(vcInfo, VCUpdateRem, spines)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
