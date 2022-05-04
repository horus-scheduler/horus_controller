package net

import context "golang.org/x/net/context"

type updateServer struct {
	updates chan *mdc_pb.MdcSessionUpdateEvent
}

func NewUpdateServer(updates chan *mdc_pb.MdcSessionUpdateEvent) *updateServer {
	return &updateServer{updates: updates}

}

func (us *updateServer) UpdateState(ctx context.Context, u *mdc_pb.MdcSessionUpdateEvent) (*mdc_pb.MdcEventResponse, error) {
	us.updates <- u
	return &mdc_pb.MdcEventResponse{Status: "OK"}, nil
}
