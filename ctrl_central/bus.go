package ctrl_central

import (
	"github.com/horus-scheduler/horus_controller/core"
	"github.com/horus-scheduler/horus_controller/core/model"
	horus_net "github.com/horus-scheduler/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

// CentralBusChan ...
type CentralBusChan struct {
	// gRPC channels
	enabledPorts  chan *horus_net.PortEnabledMessage
	disabledPorts chan *horus_net.PortDisabledMessage
}

// NewCentralBusChan ...
func NewCentralBusChan(
	enabledPorts chan *horus_net.PortEnabledMessage,
	disabledPorts chan *horus_net.PortDisabledMessage,
) *CentralBusChan {
	return &CentralBusChan{
		enabledPorts:  enabledPorts,
		disabledPorts: disabledPorts,
	}
}

// CentralBus ...
type CentralBus struct {
	*CentralBusChan
	cps      []CentralizedCP
	topology *model.Topology
	vcm      *core.VCManager
	doneChan chan bool
}

// NewCentralBus ...
func NewCentralBus(topology *model.Topology,
	vcm *core.VCManager,
	cps []CentralizedCP,
	busChan *CentralBusChan) *CentralBus {
	return &CentralBus{
		CentralBusChan: busChan,
		topology:       topology,
		vcm:            vcm,
		cps:            cps,
		doneChan:       make(chan bool, 1),
	}
}

func (e *CentralBus) getCP(asicStr string) CentralizedCP {
	for _, cp := range e.cps {
		if cp.GetAsicStr() == asicStr {
			return cp
		}
	}
	return nil
}

func (e *CentralBus) processIngress() {
	for {
		select {

		case enabledPortMsg := <-e.enabledPorts:
			if enabledPortMsg == nil {
				logrus.Error("[CentralBus] port message isn't found")
			} else {
				port := enabledPortMsg.Port
				if port != nil && port.Asic != nil {
					asicStr := port.Asic.ID
					cp := e.getCP(asicStr)
					cp.BringPortUp(port)
				} else {
					logrus.Error("[CentralBus] port isn't found")
				}
			}

		case disabledPortMsg := <-e.disabledPorts:
			if disabledPortMsg == nil {
				logrus.Error("[CentralBus] port message isn't found")
			} else {
				port := disabledPortMsg.Port
				if port != nil && port.Asic != nil {
					asicStr := port.Asic.ID
					cp := e.getCP(asicStr)
					cp.TakePortDown(port)
				} else {
					logrus.Error("[CentralBus] port isn't found")
				}
			}

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
