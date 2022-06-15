package ctrl_sw

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

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewLeafBusChan ...
func NewLeafBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	updatedServersRPC chan *core.LeafHealthMsg,
	newServersRPC chan *horus_net.ServerAddedMessage,
	asicIngress chan []byte,
	asicEgress chan []byte) *LeafBusChan {
	return &LeafBusChan{
		hmMsg:             hmIngressActiveNode,
		updatedServersRPC: updatedServersRPC,
		newServersRPC:     newServersRPC,
		asicIngress:       asicIngress,
		asicEgress:        asicEgress,
	}
}

// LeafBus ...
type LeafBus struct {
	*LeafBusChan
	healthMgr *core.LeafHealthManager
	topology  *model.Topology
	bfrt      *bfrtC.Client // BfRt client
	DoneChan  chan bool
}

// NewLeafBus ...
func NewLeafBus(busChan *LeafBusChan,
	healthMgr *core.LeafHealthManager,
	topology *model.Topology,
	bfrt *bfrtC.Client) *LeafBus {
	return &LeafBus{
		LeafBusChan: busChan,
		healthMgr:   healthMgr,
		topology:    topology,
		bfrt:        bfrt,
		DoneChan:    make(chan bool, 1),
	}
}

func (e *LeafBus) processIngress() {
	agg := make(chan *core.LeafHealthMsg)
	go func(c chan *core.LeafHealthMsg) {
		for msg := range c {
			logrus.Debug("[LeafBus] Received updated servers from health manager")
			agg <- msg
		}
	}(e.hmMsg)
	go func(c chan *core.LeafHealthMsg) {
		for msg := range c {
			logrus.Debug("[LeafBus] Received updated servers from RPC")
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
			logrus.Debug("[LeafBus] Shutting down the leaf bus")
			stop = true
		// Message about a new added server from the RPC
		case message := <-e.newServersRPC:
			go func() {
				logrus.Debugf("[LeafBus] Sending update pkt to server %d", message.Server.Id)
				e.topology.Debug()
			}()

		// Message about servers to be updated from either the health manager or the RPC
		case hmMsg := <-agg:
			go func() {
				// TODO: Complete...
				// hmMsg.Updated includes the set of servers to be updated
				// src & dst IPs, src ID, cluster ID, pkt type
				for _, server := range hmMsg.Updated {
					logrus.Debugf("[LeafBus] Sending update pkt to server %d", server.ID)
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

func (e *LeafBus) Start() {
	go e.processIngress()
}
