package net

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type spineTopologyServer struct {
	horus_pb.UnimplementedHorusTopologyServer

	failedLeavesToRPC  chan *LeafFailedMessage
	failedServersToRPC chan *ServerFailedMessage
	newServers         chan *ServerAddedMessage
	topology           *model.Topology
	vcm                *core.VCManager
}

func NewSpineTopologyServer(topology *model.Topology,
	vcm *core.VCManager,
	failedLeavesToRPC chan *LeafFailedMessage,
	failedServersToRPC chan *ServerFailedMessage,
	newServers chan *ServerAddedMessage,
) *spineTopologyServer {
	return &spineTopologyServer{
		topology:           topology,
		vcm:                vcm,
		failedLeavesToRPC:  failedLeavesToRPC,
		failedServersToRPC: failedServersToRPC,
		newServers:         newServers,
	}
}
func (s *spineTopologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debug("[SpineTopoServer] Adding a leaf")
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *spineTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	leaf := s.topology.GetNode(leafID, model.NodeType_Leaf)
	if leaf == nil {
		logrus.Debugf("[SpineTopoServer] Leaf %d doesn't exist", leafID)
		return &horus_pb.HorusResponse{Status: "FAILD"}, nil
	}
	logrus.Debugf("[SpineTopoServer] Failing a leaf %d at Spine", leafID)
	detached := s.vcm.DetachLeaf(leafID)
	leafIdx := s.topology.RemoveLeaf(leafID)
	logrus.Debugf("[SpineTopoServer] Leaf %d detached? %t, removed? %t", leafID, detached, leafIdx >= 0)
	if leafIdx >= 0 && detached {
		logrus.Debugf("[SpineTopoServer] Leaf %d is removed", leafInfo.Id)
		s.failedLeavesToRPC <- NewLeafFailedMessage(leafInfo, []*model.Node{leaf})
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "FAILD"}, nil
}

func (s *spineTopologyServer) AddServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[SpineTopoServer] Adding a server %d at Spine", serverInfo.Id)
	server, err := s.topology.AddServerToLeaf(serverInfo, uint16(serverInfo.LeafID))
	if err != nil {
		logrus.Error(err)
		return &horus_pb.HorusResponse{Status: "FAILED"}, nil
	}
	if server.Parent != nil && server.Parent.Parent != nil {
		leaf := server.Parent
		s.newServers <- NewServerAddedMessage(serverInfo, leaf)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}

	if server.Parent == nil {
		logrus.Error("[SpineTopoServer] Adding server %d failed because it doesn't belong to a leaf", server.ID)
	} else {
		if server.Parent.Parent == nil {
			logrus.Error("[SpineTopoServer] Adding server %d failed because it doesn't belong to a spine", server.ID)
		}
	}
	return &horus_pb.HorusResponse{Status: "FAILED"}, nil
}

func (s *spineTopologyServer) FailServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	serverID := uint16(serverInfo.Id)
	server := s.topology.GetNode(serverID, model.NodeType_Server)
	if server == nil {
		logrus.Debugf("[SpineTopoServer] Server %d doesn't exist", serverID)
		return &horus_pb.HorusResponse{Status: "FAILD"}, nil
	}
	logrus.Debugf("[SpineTopoServer] Failing a server %d at Spine", serverID)
	detached := s.vcm.DetachServer(serverID)
	removed, _ := s.topology.RemoveServer(serverID)
	logrus.Debugf("[SpineTopoServer] Server %d detached? %t, removed? %t", serverID, detached, removed)
	if removed && detached {
		logrus.Debugf("[SpineTopoServer] Server %d is removed", serverInfo.Id)
		s.failedServersToRPC <- NewServerFailedMessage(serverInfo, []*model.Node{server.Parent})
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "FAILD"}, nil
}
