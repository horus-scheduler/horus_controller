//go:build exclude
// +build exclude

package net

import (
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	context "golang.org/x/net/context"
)

type appServer struct {
	events chan *horus_pb.MdcAppEvent
	horus_pb.UnimplementedMdcAppNotifierServer
}

func NewAppServer(updates chan *horus_pb.MdcAppEvent) *appServer {
	return &appServer{events: updates}

}

func (us *appServer) StartSession(ctx context.Context, u *horus_pb.MdcAppEvent) (*horus_pb.MdcEventResponse, error) {
	us.events <- u
	return &horus_pb.MdcEventResponse{Status: "OK"}, nil
}

func (us *appServer) JoinSession(ctx context.Context, u *horus_pb.MdcAppEvent) (*horus_pb.MdcEventResponse, error) {
	us.events <- u
	return &horus_pb.MdcEventResponse{Status: "OK"}, nil
}

func (us *appServer) LeaveSession(ctx context.Context, u *horus_pb.MdcAppEvent) (*horus_pb.MdcEventResponse, error) {
	us.events <- u
	return &horus_pb.MdcEventResponse{Status: "OK"}, nil
}
