package net

import (
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type spineSrvServer struct {
	horus_pb.UnimplementedHorusServiceServer

	failedLeavesToRPC  chan *LeafFailedMessage
	failedServersToRPC chan *ServerFailedMessage
	newLeavesToRPC     chan *LeafAddedMessage
	newServers         chan *ServerAddedMessage
	newVCs             chan *VCUpdatedMessage
	topology           *model.Topology
	vcm                *core.VCManager
}

func NewSpineSrvServer(topology *model.Topology,
	vcm *core.VCManager,
	failedLeavesToRPC chan *LeafFailedMessage,
	failedServersToRPC chan *ServerFailedMessage,
	newLeavesToRPC chan *LeafAddedMessage,
	newServers chan *ServerAddedMessage,
	newVCs chan *VCUpdatedMessage,
) *spineSrvServer {
	return &spineSrvServer{
		topology:           topology,
		vcm:                vcm,
		failedLeavesToRPC:  failedLeavesToRPC,
		failedServersToRPC: failedServersToRPC,
		newLeavesToRPC:     newLeavesToRPC,
		newServers:         newServers,
		newVCs:             newVCs,
	}
}
func (s *spineSrvServer) AddLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[SpineTopoServer] Adding a leaf %d to spine %d", leafInfo.Id, leafInfo.SpineID)
	leaf, err := s.topology.AddLeafToSpine(leafInfo)
	if err != nil {
		logrus.Error(err)
		return &horus_pb.HorusResponse{Status: "FAILED"}, nil
	}
	if leaf.Parent != nil {
		s.newLeavesToRPC <- NewLeafAddedMessage(leafInfo, leaf)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}

	logrus.Error("[SpineTopoServer] Adding leaf %d failed because it doesn't belong to a spine", leaf.ID)
	return &horus_pb.HorusResponse{Status: "FAILED"}, nil
}

func (s *spineSrvServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
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

func (s *spineSrvServer) AddServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
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

func (s *spineSrvServer) FailServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
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

func (s *spineSrvServer) GetVCs(ctx context.Context, e *empty.Empty) (*horus_pb.VCsResponse, error) {
	return &horus_pb.VCsResponse{}, nil
}

func (s *spineSrvServer) AddVC(ctx context.Context, vcInfo *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[SpineServer] Adding VC: %d", vcInfo.Id)
	vc, err := model.NewVC(vcInfo, s.topology)
	if err != nil {
		logrus.Warnf("[SpineServer] Adding VC: %d failed: %s", vcInfo.Id, err.Error())
		return nil, err
	}
	if ok, err := s.vcm.AddVC(vc); !ok {
		return nil, err
	}

	var leaves []*model.Node
	for _, leaf := range vc.Leaves.Internal() {
		leaves = append(leaves, leaf)
	}

	s.newVCs <- NewVCUpdatedMessage(vcInfo, VCUpdateAdd, leaves)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *spineSrvServer) RemoveVC(ctx context.Context, vcInfo *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	clusterID := uint16(vcInfo.Id)
	logrus.Debugf("[SpineServer] Removing VC: %d", clusterID)

	vc := s.vcm.GetVC(clusterID)
	if vc == nil {
		logrus.Warnf("[SpineServer] Removing VC: %d failed: doesn't exist", clusterID)
		return nil, fmt.Errorf("VC %d doesn't exist", clusterID)
	}
	var leaves []*model.Node
	for _, leaf := range vc.Leaves.Internal() {
		leaves = append(leaves, leaf)
	}

	s.vcm.RemoveVC(vc)
	s.newVCs <- NewVCUpdatedMessage(vcInfo, VCUpdateRem, leaves)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
