package core

import "github.com/khaledmdiab/horus_controller/core/model"

type HealthManagerMsg struct {
	Success bool
	Updated []*model.Node
}

func NewActiveNodeMsg(success bool, updated []*model.Node) *HealthManagerMsg {
	return &HealthManagerMsg{Success: success, Updated: updated}
}
