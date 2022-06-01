package core

import "github.com/khaledmdiab/horus_controller/core/model"

type LeafHealthMsg struct {
	Success bool
	Updated []*model.Node
}

func NewLeafHealthMsg(success bool, updated []*model.Node) *LeafHealthMsg {
	return &LeafHealthMsg{Success: success, Updated: updated}
}

type SpineHealthMsg struct {
	Success bool
}

func NewSpineHealthMsg(success bool) *SpineHealthMsg {
	return &SpineHealthMsg{Success: success}
}
