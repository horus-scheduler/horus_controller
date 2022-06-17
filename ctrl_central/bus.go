package ctrl_central

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
)

// CentralBusChan ...
type CentralBusChan struct {
	// gRPC channels
}

// NewCentralBusChan ...
func NewCentralBusChan() *CentralBusChan {
	return &CentralBusChan{}
}

// CentralBus ...
type CentralBus struct {
	*CentralBusChan
	topology *model.Topology
	vcm      *core.VCManager
	doneChan chan bool
}

// NewCentralBus ...
func NewCentralBus(topology *model.Topology,
	vcm *core.VCManager,
	busChan *CentralBusChan) *CentralBus {
	return &CentralBus{
		CentralBusChan: busChan,
		topology:       topology,
		vcm:            vcm,
		doneChan:       make(chan bool, 1),
	}
}

func (e *CentralBus) processIngress() {
	for {
		select {

		// case appEvent := <-e.rpcAppIngress:
		// 	go func() {
		// 		logrus.Debug(appEvent)
		// 	}()

		default:
			continue
		}
	}
}

// Start ...
func (e *CentralBus) Start() {
	go e.processIngress()
	<-e.doneChan
}
