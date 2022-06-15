package net

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type leafTopologyServer struct {
	horus_pb.UnimplementedHorusTopologyServer

	updatedServers chan *core.LeafHealthMsg
	newServers     chan *ServerAddedMessage
	topology       *model.Topology
	vcm            *core.VCManager
}

func NewLeafTopologyServer(topology *model.Topology,
	vcm *core.VCManager,
	updatedServers chan *core.LeafHealthMsg,
	newServers chan *ServerAddedMessage,
) *leafTopologyServer {
	return &leafTopologyServer{
		topology:       topology,
		vcm:            vcm,
		updatedServers: updatedServers,
		newServers:     newServers,
	}
}
func (s *leafTopologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (s *leafTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (s *leafTopologyServer) AddServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[LeafTopoServer] Adding a server %d at Leaf", serverInfo.Id)
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
		logrus.Error("[LeafTopoServer] Adding server %d failed because it doesn't belong to a leaf", server.ID)
	} else {
		if server.Parent.Parent == nil {
			logrus.Error("[LeafTopoServer] Adding server %d failed because it doesn't belong to a spine", server.ID)
		}
	}
	return &horus_pb.HorusResponse{Status: "FAILED"}, nil
}

func (s *leafTopologyServer) FailServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	serverID := uint16(serverInfo.Id)
	logrus.Debugf("[LeafTopoServer] Failing a server %d at Leaf", serverID)
	server := s.topology.GetNode(serverID, model.NodeType_Server)
	if server == nil {
		logrus.Debugf("[LeafTopoServer] Server %d doesn't exist at Leaf", serverID)
		return &horus_pb.HorusResponse{Status: "FAILD"}, nil
	}

	// Remove the server from *all* VCs
	detached := s.vcm.DetachServer(server.ID)
	// Remove the server from the topology
	removed, updated := s.topology.RemoveServer(server.ID)

	logrus.Debugf("[LeafTopoServer] Server %d detached? %t, removed? %t", serverID, detached, removed)
	if len(updated) > 0 {
		s.updatedServers <- core.NewLeafHealthMsg(true, updated)
	}

	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
