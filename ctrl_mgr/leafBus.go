package ctrl_mgr

import (
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

// LeafBusChan ...
type LeafBusChan struct {
	hmMsg             chan *core.LeafHealthMsg           // recv-from healthManager
	updatedServersRPC chan *core.LeafHealthMsg           // recv-from RPC endpoint
	newServersRPC     chan *horus_net.ServerAddedMessage // recv-from RPC endpoint
	newVCsRPC         chan *horus_net.VCUpdatedMessage   // recv-from RPC endpoint

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewLeafBusChan ...
func NewLeafBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	updatedServersRPC chan *core.LeafHealthMsg,
	newServersRPC chan *horus_net.ServerAddedMessage,
	newVCsRPC chan *horus_net.VCUpdatedMessage,
	asicIngress chan []byte,
	asicEgress chan []byte) *LeafBusChan {
	return &LeafBusChan{
		hmMsg:             hmIngressActiveNode,
		updatedServersRPC: updatedServersRPC,
		newServersRPC:     newServersRPC,
		newVCsRPC:         newVCsRPC,
		asicIngress:       asicIngress,
		asicEgress:        asicEgress,
	}
}

// LeafBus ...
type LeafBus struct {
	*LeafBusChan
	ctrlID    uint16
	healthMgr *core.LeafHealthManager
	topology  *model.Topology
	vcm       *core.VCManager
	bfrt      *bfrtC.Client // BfRt client
	DoneChan  chan bool
}

// NewLeafBus ...
func NewBareLeafBus(ctrlID uint16, busChan *LeafBusChan,
	bfrt *bfrtC.Client) *LeafBus {
	return &LeafBus{
		LeafBusChan: busChan,
		ctrlID:      ctrlID,
		// healthMgr:   healthMgr,
		// topology:    topology,
		// vcm:         vcm,
		bfrt:     bfrt,
		DoneChan: make(chan bool, 1),
	}
}

func (e *LeafBus) SetHealthManager(healthMgr *core.LeafHealthManager) {
	if e.healthMgr == nil {
		e.healthMgr = healthMgr
	}
}

func (e *LeafBus) SetTopology(topology *model.Topology) {
	if e.topology == nil {
		e.topology = topology
	}
}

func (e *LeafBus) SetVCManager(vcm *core.VCManager) {
	if e.vcm == nil {
		e.vcm = vcm
	}
}

func (e *LeafBus) processIngress() {
	agg := make(chan *core.LeafHealthMsg)
	go func(c chan *core.LeafHealthMsg) {
		for msg := range c {
			logrus.Debugf("[LeafBus-%d] Received updated servers from health manager", e.ctrlID)
			agg <- msg
		}
	}(e.hmMsg)
	go func(c chan *core.LeafHealthMsg) {
		for msg := range c {
			logrus.Debugf("[LeafBus-%d] Received updated servers from RPC", e.ctrlID)
			agg <- msg
		}
	}(e.updatedServersRPC)

	stop := false
	for {
		if stop {
			break
		}
		select {
		case <-e.DoneChan:
			logrus.Debugf("[LeafBus-%d] Shutting down the leaf bus", e.ctrlID)
			stop = true

		// Message about a new added server from the RPC
		case message := <-e.newServersRPC:
			go func() {
				logrus.Debugf("[LeafBus-%d] Sending update pkt to server %d", e.ctrlID, message.Server.Id)
				e.topology.Debug()
			}()

		// Message about a new added VC from the RPC
		case message := <-e.newVCsRPC:
			go func() {
				if message.Type == horus_net.VCUpdateAdd {
					logrus.Debugf("[LeafBus-%d] VC %d was added", e.ctrlID, message.VCInfo.Id)
				} else if message.Type == horus_net.VCUpdateRem {
					logrus.Debugf("[LeafBus-%d] VC %d was removed", e.ctrlID, message.VCInfo.Id)
				}
				e.vcm.Debug()
			}()

		// Message about servers to be updated from either the health manager or the RPC
		case hmMsg := <-agg:
			go func() {
				// TODO: Complete...
				// hmMsg.Updated includes the set of servers to be updated
				// src & dst IPs, src ID, cluster ID, pkt type
				for _, server := range hmMsg.Updated {
					logrus.Debugf("[LeafBus-%d] Sending update pkt to server %d", e.ctrlID, server.ID)
					horusPkt := &horus_net.HorusPacket{
						PktType:    horus_net.PKT_TYPE_KEEP_ALIVE,
						ClusterID:  0xffff,
						SrcID:      0xffff,
						DstID:      server.ID,
						SeqNum:     0,
						RestOfData: []byte{0x00},
					}
					pktBytes, err := horus_net.CreateFullHorusPacket(horusPkt,
						net.IP{1, 1, 1, 1},
						net.IP{2, 2, 2, 2})
					if err != nil {
						e.asicEgress <- pktBytes
					}
				}
			}()

		// Packet from the ASIC
		case dpMsg := <-e.asicIngress:
			go func() {
				pkt := gopacket.NewPacket(dpMsg, layers.LayerTypeEthernet, gopacket.Default)
				if horusLayer := pkt.Layer(horus_net.LayerTypeHorus); horusLayer != nil {
					// Get actual pkt
					horusPkt, _ := horusLayer.(*horus_net.HorusPacket)
					logrus.Debug(horusPkt)
					// TODO: Which pkt type indicates a ping from the client?
					if horusPkt.PktType == horus_net.PKT_TYPE_WORKER_ID {
						// Start the Ping-pong protocol
						nodeID := horusPkt.SrcID
						// Update the health manager
						e.healthMgr.OnNodePingRecv(nodeID, 0)

						// Send the Pong pkt?
						// TODO: modify the index (zero) and pkt type if needed
						newPktBytes := pkt.Data()
						newPktBytes[0] = byte(horus_net.PKT_TYPE_WORKER_ID_ACK)
						e.asicEgress <- newPktBytes
					}
					// TODO: do we need to process receiving other pkt types?
				}
			}()

		default:
			continue
		}
	}
}

func (e *LeafBus) initialize() {
	logrus.Infof("[LeafBus-%d] Running initialization logic", e.ctrlID)
	e.topology.Debug()
	e.vcm.Debug()
}

func (e *LeafBus) Start() {
	e.initialize()
	go e.processIngress()
}
