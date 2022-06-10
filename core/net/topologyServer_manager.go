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
	rpcEgress chan *LeafFailedMessage
	topology  *model.Topology
	vcm       *core.VCManager
}

func NewManagerTopologyServer(rpcEgress chan *LeafFailedMessage) *managerTopologyServer {
	return &managerTopologyServer{topology: nil, vcm: nil, rpcEgress: rpcEgress}
}
func (s *managerTopologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debug("Adding a leaf")
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("Failing a leaf %d @Manager", leafID)
	s.rpcEgress <- NewLeafFailedMessage(leafInfo, nil)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerTopologyServer) AddServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *managerTopologyServer) FailServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
