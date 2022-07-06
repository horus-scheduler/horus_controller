package core

import (
	"github.com/horus-scheduler/horus_controller/core/model"
)

type SpineHealthManager struct {
	hmEgress chan *SpineHealthMsg
	topology *model.Topology
	vcMgr    *VCManager
	doneChan chan bool
}

func NewSpineHealthManager(hmEgress chan *SpineHealthMsg,
	topology *model.Topology,
	vcMgr *VCManager,
	healthyNodeTimeOut int64) (*SpineHealthManager, error) {

	return &SpineHealthManager{
		hmEgress: hmEgress,
		topology: topology,
		vcMgr:    vcMgr,
		doneChan: make(chan bool),
	}, nil
}

// Logic for handling when a leaf fails
func (hm *SpineHealthManager) OnLeafFailed(leafID uint16) {

}

func (hm *SpineHealthManager) Start() {
	<-hm.doneChan
}
