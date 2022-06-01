package net

import (
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	context "golang.org/x/net/context"
)

type vcServer struct {
	horus_pb.UnimplementedHorusVCServer
}

func NewTorServer() *vcServer {
	return &vcServer{}
}

func (v *vcServer) AddVC(context.Context, *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (v *vcServer) RemoveVC(context.Context, *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
