package net

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type LeafFailedMessage struct {
	leaf *horus_pb.LeafInfo
	dsts []*model.Node
}

func NewLeafFailedMessage(leaf *horus_pb.LeafInfo, dsts []*model.Node) *LeafFailedMessage {
	return &LeafFailedMessage{leaf: leaf, dsts: dsts}
}

type centralTopologyServer struct {
	horus_pb.UnimplementedHorusTopologyServer

	failedLeaves chan *LeafFailedMessage
	topology     *model.Topology
	vcm          *core.VCManager
}

func NewCentralTopologyServer(topology *model.Topology,
	vcm *core.VCManager,
	failedLeaves chan *LeafFailedMessage) *centralTopologyServer {
	return &centralTopologyServer{topology: topology, vcm: vcm, failedLeaves: failedLeaves}
}
func (s *centralTopologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debug("Adding a leaf")
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *centralTopologyServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	leafID := uint16(leafInfo.Id)
	logrus.Debugf("Failing a leaf %d @Central", leafID)
	vcs, ok := s.vcm.GetVCsOfLeaf(leafID)

	// Spines to be updated
	spinesMap := model.NewNodeMap()
	if ok {
		for _, vc := range vcs {
			for spineId, spine := range vc.Spines.Internal() {
				spinesMap.Store(spineId, spine)
			}
		}
	}
	spines := make([]*model.Node, 0)
	for _, spine := range spinesMap.Internal() {
		spines = append(spines, spine)
	}
	leaf := s.topology.GetNode(leafID, model.NodeType_Leaf)
	detached := s.vcm.DetachLeaf(leafID)
	leafIdx := s.topology.RemoveLeaf(leafID)
	if leaf != nil && leafIdx >= 0 && detached {
		s.failedLeaves <- NewLeafFailedMessage(leafInfo, spines)
		return &horus_pb.HorusResponse{Status: "OK"}, nil
	}
	return &horus_pb.HorusResponse{Status: "FAILD"}, nil
}

func (s *centralTopologyServer) AddServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *centralTopologyServer) FailServer(ctx context.Context, server *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
