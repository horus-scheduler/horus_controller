package net

import (
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	context "golang.org/x/net/context"
)

type topologyServer struct {
	// updates chan *horus_pb.MdcSessionUpdateEvent
	horus_pb.UnimplementedHorusTopologyServer
}

func NewTopologyServer() *topologyServer {
	return &topologyServer{}
}
func (s *topologyServer) AddLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *topologyServer) FailLeaf(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *topologyServer) AddServer(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (s *topologyServer) FailServer(ctx context.Context, leaf *horus_pb.LeafInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
