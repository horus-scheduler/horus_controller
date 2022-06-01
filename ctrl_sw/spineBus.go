package ctrl_sw

import (
	"github.com/khaledmdiab/horus_controller/core"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
)

// SpineBusChan ...
type SpineBusChan struct {
	// healthManager channels
	hmIngressActiveNode chan *core.LeafHealthMsg // recv-from healthManager
	// send-to healthManager

	// gRPC channels
	rpcIngress chan *horus_pb.HorusResponse // recv-from gRPC connection
	rpcEgress  chan *horus_pb.HorusResponse // send-to gRPC client

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewSpineBusChan ...
func NewSpineBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	rpcIngress chan *horus_pb.HorusResponse,
	rpcEgress chan *horus_pb.HorusResponse,
	asicIngress chan []byte,
	asicEgress chan []byte) *SpineBusChan {
	return &SpineBusChan{
		hmIngressActiveNode: hmIngressActiveNode,
		rpcIngress:          rpcIngress,
		rpcEgress:           rpcEgress,
		asicIngress:         asicIngress,
		asicEgress:          asicEgress,
	}
}

// SpineBus ...
type SpineBus struct {
	*SpineBusChan
	healthMgr *core.LeafHealthManager
	doneChan  chan bool
}

// NewSpineBus ...
func NewSpineBus(busChan *SpineBusChan,
	healthMgr *core.LeafHealthManager) *SpineBus {
	return &SpineBus{
		SpineBusChan: busChan,
		healthMgr:    healthMgr,
		doneChan:     make(chan bool, 1),
	}
}

func (e *SpineBus) processIngress() {
	for {
		select {
		case message := <-e.rpcIngress:
			// TODO: receives a msg that a leaf had failed
			go func() {
				logrus.Debug(message)
			}()

		case activeNodeMsg := <-e.hmIngressActiveNode:
			logrus.Debugf("Send set-active-agent to switch %v", activeNodeMsg)

		default:
			continue
		}
	}
}

func (e *SpineBus) Start() {
	go e.processIngress()
	<-e.doneChan
}
