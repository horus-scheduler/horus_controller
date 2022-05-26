package ctrl_sw

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/khaledmdiab/horus_controller/core"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
)

// SpineEventEncDecChan ...
type SpineEventEncDecChan struct {
	// healthManager channels
	hmIngressActiveNode chan *core.HealthManagerMsg // recv-from healthManager
	// send-to healthManager

	// gRPC channels
	rpcIngress chan *horus_pb.HorusMessage // recv-from gRPC connection
	rpcEgress  chan *horus_pb.HorusMessage // send-to gRPC client

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewSpineEventEncDecChan ...
func NewSpineEventEncDecChan(hmIngressActiveNode chan *core.HealthManagerMsg,
	rpcIngress chan *horus_pb.HorusMessage,
	rpcEgress chan *horus_pb.HorusMessage,
	asicIngress chan []byte,
	asicEgress chan []byte) *SpineEventEncDecChan {
	return &SpineEventEncDecChan{
		hmIngressActiveNode: hmIngressActiveNode,
		rpcIngress:          rpcIngress,
		rpcEgress:           rpcEgress,
		asicIngress:         asicIngress,
		asicEgress:          asicEgress,
	}
}

// SpineEventEncDec ...
type SpineEventEncDec struct {
	*SpineEventEncDecChan
	healthMgr *core.NodeHealthManager
	doneChan  chan bool
}

// NewSpineEventEncDec ...
func NewSpineEventEncDec(encDecChan *SpineEventEncDecChan,
	healthMgr *core.NodeHealthManager) *SpineEventEncDec {
	return &SpineEventEncDec{
		SpineEventEncDecChan: encDecChan,
		healthMgr:            healthMgr,
		doneChan:             make(chan bool, 1),
	}
}

func (e *SpineEventEncDec) processIngress() {
	for {
		select {
		case message := <-e.rpcIngress:
			go func() {
				logrus.Debug(message)
			}()

		case activeNodeMsg := <-e.hmIngressActiveNode:
			logrus.Debugf("Send set-active-agent to switch %v", activeNodeMsg)

		case dpMsg := <-e.asicIngress:
			go func() {
				pkt := gopacket.NewPacket(dpMsg, layers.LayerTypeEthernet, gopacket.Default)
				if horusLayer := pkt.Layer(horus_net.LayerTypeHorus); horusLayer != nil {
					// Get actual Mdc data from this layer
					horusPkt, _ := horusLayer.(*horus_net.HorusPacket)
					logrus.Debug(horusPkt)
				}
			}()

		default:
			continue
		}
	}
}

func (e *SpineEventEncDec) Start() {
	go e.processIngress()
	<-e.doneChan
}
