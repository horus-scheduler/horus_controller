package net

import context "golang.org/x/net/context"
import "github.com/khaledmdiab/horus_controller/protobuf"

type appServer struct {
	events chan *mdc_pb.MdcAppEvent
}

func NewAppServer(updates chan *mdc_pb.MdcAppEvent) *appServer {
	return &appServer{events: updates}

}

func (us *appServer) StartSession(ctx context.Context, u *mdc_pb.MdcAppEvent) (*mdc_pb.MdcEventResponse, error) {
	us.events <- u
	return &mdc_pb.MdcEventResponse{Status: "OK"}, nil
}

func (us *appServer) JoinSession(ctx context.Context, u *mdc_pb.MdcAppEvent) (*mdc_pb.MdcEventResponse, error) {
	us.events <- u
	return &mdc_pb.MdcEventResponse{Status: "OK"}, nil
}

func (us *appServer) LeaveSession(ctx context.Context, u *mdc_pb.MdcAppEvent) (*mdc_pb.MdcEventResponse, error) {
	us.events <- u
	return &mdc_pb.MdcEventResponse{Status: "OK"}, nil
}
