package net

import (
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type managerSrvServer struct {
	horus_pb.UnimplementedHorusServiceServer
	failedLeaves chan *LeafFailedMessage
	newLeaves    chan *LeafAddedMessage
	topology     *model.Topology
	vcm          *core.VCManager
}

func NewManagerSrvServer(
	failedLeaves chan *LeafFailedMessage,
	newLeaves chan *LeafAddedMessage,
) *managerSrvServer {
	return &managerSrvServer{topology: nil, vcm: nil,
		failedLeaves: failedLeaves,
		newLeaves:    newLeaves,
	}
}
func (s *managerSrvServer) AddLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("[ManagerTopoServer] Add leaf %d", leafID)
	s.newLeaves <- NewLeafAddedMessage(leafInfo, nil)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerSrvServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("[ManagerTopoServer] Failing leaf %d", leafID)
	s.failedLeaves <- NewLeafFailedMessage(leafInfo, nil)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerSrvServer) AddServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (s *managerSrvServer) FailServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (v *managerSrvServer) GetVCs(ctx context.Context, e *empty.Empty) (*horus_pb.VCsResponse, error) {
	return &horus_pb.VCsResponse{}, nil
}

func (v *managerSrvServer) AddVC(context.Context, *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (v *managerSrvServer) RemoveVC(context.Context, *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
