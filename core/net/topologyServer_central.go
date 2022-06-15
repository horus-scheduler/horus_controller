package net

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type LeafFailedMessage struct {
	Leaf *horus_pb.LeafInfo
	Dsts []*model.Node
}

type ServerFailedMessage struct {
	Server *horus_pb.ServerInfo
	Dsts   []*model.Node
}

type ServerAddedMessage struct {
	Server *horus_pb.ServerInfo
	Dst    *model.Node
}

func NewLeafFailedMessage(leaf *horus_pb.LeafInfo, dsts []*model.Node) *LeafFailedMessage {
	return &LeafFailedMessage{Leaf: leaf, Dsts: dsts}
}

func NewServerFailedMessage(server *horus_pb.ServerInfo, dsts []*model.Node) *ServerFailedMessage {
	return &ServerFailedMessage{Server: server, Dsts: dsts}
}

func NewServerAddedMessage(server *horus_pb.ServerInfo, dst *model.Node) *ServerAddedMessage {
	return &ServerAddedMessage{Server: server, Dst: dst}
}

type centralTopologyServer struct {
	horus_pb.UnimplementedHorusTopologyServer

	failedLeaves  chan *LeafFailedMessage
	failedServers chan *ServerFailedMessage
	newServers    chan *ServerAddedMessage
	topology      *model.Topology
	vcm           *core.VCManager
}

func NewCentralTopologyServer(topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage,
	failedServers chan *ServerFailedMessage,
	newServers chan *ServerAddedMessage,
) *centralTopologyServer {
	return &centralTopologyServer{
		topology:      topology,
		vcm:           vcm,
		failedLeaves:  failedLeaves,
		failedServers: failedServers,
		newServers:    newServers,
	}
}
func (s *centralTopologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debug("[CentralTopoServer] Adding a leaf")
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *centralTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("[CentralTopoServer] Failing a leaf %d at Central", leafID)
	leaf := s.topology.GetNode(leafID, model.NodeType_Leaf)
	if leaf == nil {
		logrus.Debugf("[CentralTopoServer] Leaf %d doesn't exist at Central", leafID)
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

func (s *centralTopologyServer) AddServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[CentralTopoServer] Adding a server %d at Central", serverInfo.Id)
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
		logrus.Error("[CentralTopoServer] Adding server %d failed because it doesn't belong to a leaf", server.ID)
	} else {
		if server.Parent.Parent == nil {
			logrus.Error("[CentralTopoServer] Adding server %d failed because it doesn't belong to a spine", server.ID)
		}
	}
	return &horus_pb.HorusResponse{Status: "FAILED"}, nil
}

func (s *centralTopologyServer) FailServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	serverID := uint16(serverInfo.Id)
	logrus.Debugf("[CentralTopoServer] Failing a server %d at Central", serverID)
	server := s.topology.GetNode(serverID, model.NodeType_Server)
	if server == nil {
		logrus.Debugf("[CentralTopoServer] Server %d doesn't exist at Central", serverID)
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
	logrus.Debugf("[CentralTopoServer] Spines count: %d", len(spines))

	detached := s.vcm.DetachServer(serverID)
	removed, _ := s.topology.RemoveServer(serverID)
	logrus.Debugf("[CentralTopoServer] Server %d detached? %t, removed? %t", serverID, detached, removed)
	if removed && detached {
		s.failedServers <- NewServerFailedMessage(serverInfo, spines)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}