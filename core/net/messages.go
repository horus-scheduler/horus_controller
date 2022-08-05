package net

import (
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
)

type VCUpdateType uint32

const (
	VCUpdateAdd VCUpdateType = iota
	VCUpdateRem
)

type LeafUpdatedMessage struct {
	Leaves []*model.Node
}

type LeafFailedMessage struct {
	Leaf *horus_pb.LeafInfo
	Dsts []*model.Node
}

type ServerFailedMessage struct {
	Server *horus_pb.ServerInfo
	Dsts   []*model.Node
}

type ServerAddedMessage struct {
	Server *horus_pb.ServerInfo
	Dst    *model.Node
}

type LeafAddedMessage struct {
	Leaf *horus_pb.LeafInfo
	Dst  *model.Node
}

type PortDisabledMessage struct {
	Port *model.Port
}

type PortEnabledMessage struct {
	Port *model.Port
}

type VCUpdatedMessage struct {
	VCInfo *horus_pb.VCInfo
	Dsts   []*model.Node
	Type   VCUpdateType
}

func NewLeafUpdatedMessage(leaves []*model.Node) *LeafUpdatedMessage {
	return &LeafUpdatedMessage{Leaves: leaves}
}

func NewLeafFailedMessage(leaf *horus_pb.LeafInfo, dsts []*model.Node) *LeafFailedMessage {
	return &LeafFailedMessage{Leaf: leaf, Dsts: dsts}
}

func NewServerFailedMessage(server *horus_pb.ServerInfo, dsts []*model.Node) *ServerFailedMessage {
	return &ServerFailedMessage{Server: server, Dsts: dsts}
}

func NewServerAddedMessage(server *horus_pb.ServerInfo, dst *model.Node) *ServerAddedMessage {
	return &ServerAddedMessage{Server: server, Dst: dst}
}

func NewLeafAddedMessage(leaf *horus_pb.LeafInfo, dst *model.Node) *LeafAddedMessage {
	return &LeafAddedMessage{Leaf: leaf, Dst: dst}
}

func NewVCUpdatedMessage(vcInfo *horus_pb.VCInfo, vcType VCUpdateType, dsts []*model.Node) *VCUpdatedMessage {
	return &VCUpdatedMessage{VCInfo: vcInfo, Type: vcType, Dsts: dsts}
}
