package net

import (
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	context "golang.org/x/net/context"
)

type updateServer struct {
	updates chan *horus_pb.MdcSessionUpdateEvent
	horus_pb.UnimplementedMdcSessionUpdaterServer
}

func NewUpdateServer(updates chan *horus_pb.MdcSessionUpdateEvent) *updateServer {
	return &updateServer{updates: updates}

}

func (us *updateServer) UpdateState(ctx context.Context, u *horus_pb.MdcSessionUpdateEvent) (*horus_pb.MdcEventResponse, error) {
	us.updates <- u
	return &horus_pb.MdcEventResponse{Status: "OK"}, nil
}
