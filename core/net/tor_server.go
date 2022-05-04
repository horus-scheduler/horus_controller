package net

import context "golang.org/x/net/context"

type torServer struct {
	events chan *mdc_pb.MdcSyncEvent
}

func NewTorServer(updates chan *mdc_pb.MdcSyncEvent) *torServer {
	return &torServer{events: updates}
}

func (us *torServer) SyncState(ctx context.Context, u *mdc_pb.MdcSyncEvent) (*mdc_pb.MdcEventResponse, error) {
	us.events <- u
	return &mdc_pb.MdcEventResponse{Status: "OK"}, nil
}

func (us *torServer) SyncDone(ctx context.Context, u *mdc_pb.MdcSyncEvent) (*mdc_pb.MdcEventResponse, error) {
	us.events <- u
	return &mdc_pb.MdcEventResponse{Status: "OK"}, nil
}
