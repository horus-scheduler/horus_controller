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

	failedLeaves      chan *LeafFailedMessage
	failedLeavesToRPC chan *LeafFailedMessage
	topology          *model.Topology
	vcm               *core.VCManager
}

func NewSpineTopologyServer(topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage,
	failedLeavesToRPC chan *LeafFailedMessage,
) *spineTopologyServer {
	return &spineTopologyServer{
		topology:          topology,
		vcm:               vcm,
		failedLeaves:      failedLeaves,
		failedLeavesToRPC: failedLeavesToRPC,
	}
}
func (s *spineTopologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debug("Adding a leaf")
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *spineTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	leaf := s.topology.GetNode(leafID, model.NodeType_Leaf)
	if leaf == nil {
		return &horus_pb.HorusResponse{Status: "FAILD"}, nil
	}
	logrus.Debugf("Failing a leaf %d @Spine", leafID)
	detached := s.vcm.DetachLeaf(leafID)
	leafIdx := s.topology.RemoveLeaf(leafID)
	logrus.Debug(detached, leafIdx)
	if leafIdx >= 0 && detached {
		logrus.Debug("REMOVED!", leafInfo.Id)
		s.failedLeavesToRPC <- NewLeafFailedMessage(leafInfo, []*model.Node{leaf})
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "FAILD"}, nil
}

func (s *spineTopologyServer) AddServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *spineTopologyServer) FailServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
