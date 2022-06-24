package net

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type leafSrvServer struct {
	horus_pb.UnimplementedHorusServiceServer

	updatedServers chan *core.LeafHealthMsg
	newServers     chan *ServerAddedMessage
	newVCs         chan *VCUpdatedMessage
	topology       *model.Topology
	vcm            *core.VCManager
}

func NewLeafSrvServer(topology *model.Topology,
	vcm *core.VCManager,
	updatedServers chan *core.LeafHealthMsg,
	newServers chan *ServerAddedMessage,
	newVCs chan *VCUpdatedMessage,
) *leafSrvServer {
	return &leafSrvServer{
		topology:       topology,
		vcm:            vcm,
		updatedServers: updatedServers,
		newServers:     newServers,
		newVCs:         newVCs,
	}
}

func (s *leafSrvServer) GetTopology(context.Context, *empty.Empty) (*horus_pb.TopoInfo, error) {
	return nil, errors.New("GetTopology isn't supported by leaf")
}

func (s *leafSrvServer) GetTopologyAtLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.TopoInfo, error) {
	return nil, errors.New("GetTopologyAtLeaf isn't supported by leaf")
}

func (s *leafSrvServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (s *leafSrvServer) FailLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "NOT_SUPPORTED"}, nil
}

func (s *leafSrvServer) AddServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
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

func (s *leafSrvServer) FailServer(ctx context.Context, serverInfo *horus_pb.ServerInfo) (*horus_pb.HorusResponse, error) {
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

	s.vcm.Debug()

	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *leafSrvServer) GetVCs(ctx context.Context, e *empty.Empty) (*horus_pb.VCsResponse, error) {
	return &horus_pb.VCsResponse{}, nil
}

func (s *leafSrvServer) GetVCsOfLeaf(ctx context.Context, leafInfo *horus_pb.LeafInfo) (*horus_pb.VCsResponse, error) {
	return &horus_pb.VCsResponse{}, nil
}

func (s *leafSrvServer) AddVC(ctx context.Context, vcInfo *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	logrus.Debugf("[LeafServer] Adding VC: %d", vcInfo.Id)
	vc, err := model.NewVC(vcInfo, s.topology)
	if err != nil {
		logrus.Warnf("[LeafServer] Adding VC: %d failed: %s", vcInfo.Id, err.Error())
		return nil, err
	}
	if ok, err := s.vcm.AddVC(vc); !ok {
		return nil, err
	}

	s.newVCs <- NewVCUpdatedMessage(vcInfo, VCUpdateAdd, nil)
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *leafSrvServer) RemoveVC(ctx context.Context, vcInfo *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	clusterID := uint16(vcInfo.Id)
	logrus.Debugf("[LeafServer] Removing VC: %d", clusterID)

	vc := s.vcm.GetVC(clusterID)
	if vc == nil {
		logrus.Warnf("[LeafServer] Removing VC: %d failed: doesn't exist", clusterID)
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
