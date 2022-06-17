package net

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type managerTopologyServer struct {
	horus_pb.UnimplementedHorusTopologyServer
	failedLeaves chan *LeafFailedMessage
	newLeaves    chan *LeafAddedMessage
	topology     *model.Topology
	vcm          *core.VCManager
}

func NewManagerTopologyServer(
	failedLeaves chan *LeafFailedMessage,
	newLeaves chan *LeafAddedMessage,
) *managerTopologyServer {
	return &managerTopologyServer{topology: nil, vcm: nil,
		failedLeaves: failedLeaves,
		newLeaves:    newLeaves,
	}
}
func (s *managerTopologyServer) AddLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("[ManagerTopoServer] Add leaf %d", leafID)
	s.newLeaves <- NewLeafAddedMessage(leafInfo, nil)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("[ManagerTopoServer] Failing leaf %d", leafID)
	s.failedLeaves <- NewLeafFailedMessage(leafInfo, nil)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerTopologyServer) AddServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (s *managerTopologyServer) FailServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}
