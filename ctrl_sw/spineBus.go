package ctrl_sw

import (
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

// SpineBusChan ...
type SpineBusChan struct {
	// healthManager channels
	hmIngressActiveNode chan *core.LeafHealthMsg // recv-from healthManager
	// send-to healthManager

	// gRPC channels
	rpcFailedLeaves  chan *horus_net.LeafFailedMessage   // recv-from gRPC
	rpcFailedServers chan *horus_net.ServerFailedMessage // recv-from gRPC
	newServers       chan *horus_net.ServerAddedMessage

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewSpineBusChan ...
func NewSpineBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	rpcFailedLeaves chan *horus_net.LeafFailedMessage,
	rpcFailedServers chan *horus_net.ServerFailedMessage,
	newServers chan *horus_net.ServerAddedMessage,
	asicIngress chan []byte,
	asicEgress chan []byte) *SpineBusChan {
	return &SpineBusChan{
		hmIngressActiveNode: hmIngressActiveNode,
		rpcFailedLeaves:     rpcFailedLeaves,
		rpcFailedServers:    rpcFailedServers,
		newServers:          newServers,
		asicIngress:         asicIngress,
		asicEgress:          asicEgress,
	}
}

// SpineBus ...
type SpineBus struct {
	*SpineBusChan
	topology  *model.Topology
	healthMgr *core.LeafHealthManager
	bfrt      *bfrtC.Client // BfRt client
	doneChan  chan bool
}

// NewSpineBus ...
func NewSpineBus(busChan *SpineBusChan,
	topology *model.Topology,
	healthMgr *core.LeafHealthManager,
	bfrt *bfrtC.Client) *SpineBus {
	return &SpineBus{
		SpineBusChan: busChan,
		topology:     topology,
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
				logrus.Debugf("[SpineBus] Using BfRt Client to remove leaf-related DP info from spine; leafID = %d", message.Leaf.Id)
			}()
		case message := <-bus.rpcFailedServers:
			// TODO: receives a msg that a server had failed
			// Notice: At this stage, the failed server has already been removed and detached
			go func() {
				logrus.Debugf("[SpineBus] Using BfRt Client to remove server-related DP info from spine; serverID = %d", message.Server.Id)
			}()
		case message := <-bus.newServers:
			// TODO: receives a msg that a server was added
			// Notice: At this stage, the added server has already been added to the topology
			go func() {
				logrus.Debugf("[SpineBus] Using BfRt Client to add server-related DP info at spine; serverID = %d", message.Server.Id)
				if bus.topology != nil {
					bus.topology.Debug()
				}
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
