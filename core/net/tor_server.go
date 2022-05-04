package net

import (
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	context "golang.org/x/net/context"
)

type torServer struct {
	events chan *horus_pb.MdcSyncEvent
	horus_pb.UnimplementedMdcControllerNotifierServer
}

func NewTorServer(updates chan *horus_pb.MdcSyncEvent) *torServer {
	return &torServer{events: updates}
}

func (us *torServer) SyncState(ctx context.Context, u *horus_pb.MdcSyncEvent) (*horus_pb.MdcEventResponse, error) {
	us.events <- u
	return &horus_pb.MdcEventResponse{Status: "OK"}, nil
}

func (us *torServer) SyncDone(ctx context.Context, u *horus_pb.MdcSyncEvent) (*horus_pb.MdcEventResponse, error) {
	us.events <- u
	return &horus_pb.MdcEventResponse{Status: "OK"}, nil
}
