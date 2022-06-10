package net

import (
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/khaledmdiab/horus_controller/core"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	context "golang.org/x/net/context"
)

type centralVCServer struct {
	horus_pb.UnimplementedHorusVCServer
	vcm *core.VCManager
}

func NewCentralVCServer(vcm *core.VCManager) *centralVCServer {
	return &centralVCServer{vcm: vcm}
}

func (v *centralVCServer) GetVCs(context.Context, *empty.Empty) (*horus_pb.VCsResponse, error) {
	logrus.Debug("GetVCs() @Central")
	vcs := v.vcm.GetVCs()
	rsp := &horus_pb.VCsResponse{}
	for _, vc := range vcs {
		vcInfo := &horus_pb.VCInfo{}
		vcInfo.Id = uint32(vc.ClusterID)
		for _, server := range vc.Servers.Internal() {
			serverInfo := &horus_pb.VCServerInfo{}
			serverInfo.Id = uint32(server.ID)
			vcInfo.Servers = append(vcInfo.Servers, serverInfo)
		}
		for _, spine := range vc.Spines.Internal() {
			vcInfo.Spines = append(vcInfo.Spines, uint32(spine.ID))
		}
		rsp.Vcs = append(rsp.Vcs, vcInfo)
	}
	return rsp, nil
}

func (v *centralVCServer) AddVC(context.Context, *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}

func (v *centralVCServer) RemoveVC(context.Context, *horus_pb.VCInfo) (*horus_pb.HorusResponse, error) {
	return &horus_pb.HorusResponse{Status: "OK"}, nil
}
