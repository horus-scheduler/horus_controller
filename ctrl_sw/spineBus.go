package ctrl_sw

import (
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/khaledmdiab/horus_controller/core"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

// SpineBusChan ...
type SpineBusChan struct {
	// healthManager channels
	hmIngressActiveNode chan *core.LeafHealthMsg // recv-from healthManager
	// send-to healthManager

	// gRPC channels
	rpcFailedLeaves chan *horus_net.LeafFailedMessage // recv-from gRPC

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewSpineBusChan ...
func NewSpineBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	rpcFailedLeaves chan *horus_net.LeafFailedMessage,
	asicIngress chan []byte,
	asicEgress chan []byte) *SpineBusChan {
	return &SpineBusChan{
		hmIngressActiveNode: hmIngressActiveNode,
		rpcFailedLeaves:     rpcFailedLeaves,
		asicIngress:         asicIngress,
		asicEgress:          asicEgress,
	}
}

// SpineBus ...
type SpineBus struct {
	*SpineBusChan
	healthMgr *core.LeafHealthManager
	bfrt      *bfrtC.Client // BfRt client
	doneChan  chan bool
}

// NewSpineBus ...
func NewSpineBus(busChan *SpineBusChan,
	healthMgr *core.LeafHealthManager,
	bfrt *bfrtC.Client) *SpineBus {
	return &SpineBus{
		SpineBusChan: busChan,
		healthMgr:    healthMgr,
		bfrt:         bfrt,
		doneChan:     make(chan bool, 1),
	}
}

func (bus *SpineBus) processIngress() {
	for {
		select {
		case message := <-bus.rpcFailedLeaves:
			// TODO: receives a msg that a leaf had failed
			// Notice: At this stage, the failed leaf has already been removed and detached
			go func() {
				logrus.Debugf("Using BfRt Client to remove spine DP info about leaf %d",
					message.Leaf.Id)
			}()

		default:
			continue
		}
	}
}

func (e *SpineBus) Start() {
	go e.processIngress()
	<-e.doneChan
}
