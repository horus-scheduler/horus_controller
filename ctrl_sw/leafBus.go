package ctrl_sw

import (
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/khaledmdiab/horus_controller/core"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

// LeafBusChan ...
type LeafBusChan struct {
	// healthManager channels
	hmMsg chan *core.LeafHealthMsg // recv-from healthManager
	// send-to healthManager

	// gRPC channels
	// rpcIngress chan *horus_pb.HorusResponse // recv-from gRPC connection
	// rpcEgress  chan *horus_pb.HorusResponse // send-to gRPC client

	// ASIC channels
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// NewLeafBusChan ...
func NewLeafBusChan(hmIngressActiveNode chan *core.LeafHealthMsg,
	asicIngress chan []byte,
	asicEgress chan []byte) *LeafBusChan {
	return &LeafBusChan{
		hmMsg:       hmIngressActiveNode,
		asicIngress: asicIngress,
		asicEgress:  asicEgress,
	}
}

// LeafBus ...
type LeafBus struct {
	*LeafBusChan
	healthMgr *core.LeafHealthManager
	bfrt      *bfrtC.Client // BfRt client
	DoneChan  chan bool
}

// NewLeafBus ...
func NewLeafBus(busChan *LeafBusChan,
	healthMgr *core.LeafHealthManager,
	bfrt *bfrtC.Client) *LeafBus {
	return &LeafBus{
		LeafBusChan: busChan,
		healthMgr:   healthMgr,
		bfrt:        bfrt,
		DoneChan:    make(chan bool, 1),
	}
}

func (e *LeafBus) processIngress() {
	stop := false
	for {
		if stop {
			break
		}
		select {
		case <-e.DoneChan:
			logrus.Debug("Shutting down the leaf bus")
			stop = true
		// Message from the RPC endpoint
		// case message := <-e.rpcIngress:
		// 	go func() {
		// 		logrus.Debug(message)
		// 	}()

		// Message from the health manager
		case hmMsg := <-e.hmMsg:
			go func() {
				logrus.Debugf("Message from the health manager %v", hmMsg)
				// TODO: Complete...
				// hmMsg.Updated includes the set of servers to be updated
				// src & dst IPs, src ID, cluster ID, pkt type
				for _, server := range hmMsg.Updated {
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
