package ctrl_sw

import (
	"encoding/binary"
	"log"
	"strconv"
	"unsafe"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/khaledmdiab/horus_controller/core"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/khaledmdiab/horus_controller/core/sequencer"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
)

// EventEncDecChan ...
type EventEncDecChan struct {
	// eventSequencer channels
	// recv-from eventSequencer
	esIngress chan *core.SyncJobResult
	// send-to eventSequencer
	esEgress chan *core.SyncJob

	// healthManager channels
	// recv-from healthManager
	hmIngressActiveNode chan *core.ActiveNodeMsg
	// send-to healthManager

	// gRPC channels
	// recv-from gRPC connection
	rpcIngress chan *horus_pb.MdcSessionUpdateEvent
	// send-to gRPC client
	rpcEgress chan *horus_pb.MdcSyncEvent

	// unixSock channels
	// recv-from unixSock connection
	localSockIngress chan []byte
	// send-to unixSock client
	localSockEgress chan []byte
}

// NewEventEncDecChan ...
func NewEventEncDecChan(esIngress chan *core.SyncJobResult,
	esEgress chan *core.SyncJob,
	hmIngressActiveNode chan *core.ActiveNodeMsg,
	rpcIngress chan *horus_pb.MdcSessionUpdateEvent,
	rpcEgress chan *horus_pb.MdcSyncEvent,
	localSockIngress chan []byte,
	localSockEgress chan []byte) *EventEncDecChan {
	return &EventEncDecChan{
		esEgress:            esEgress,
		esIngress:           esIngress,
		hmIngressActiveNode: hmIngressActiveNode,
		rpcIngress:          rpcIngress,
		rpcEgress:           rpcEgress,
		localSockIngress:    localSockIngress,
		localSockEgress:     localSockEgress,
	}
}

// EventEncDec ...
type EventEncDec struct {
	*EventEncDecChan
	healthMgr *core.NodeHealthManager
	es        *sequencer.SimpleEventSequencer
	torId     uint32
	doneChan  chan bool
}

// NewEventEncDec ...
func NewEventEncDec(encDecChan *EventEncDecChan,
	healthMgr *core.NodeHealthManager,
	eventSequencer *sequencer.SimpleEventSequencer, torID uint32) *EventEncDec {
	return &EventEncDec{
		EventEncDecChan: encDecChan,
		healthMgr:       healthMgr,
		es:              eventSequencer,
		torId:           torID,
		doneChan:        make(chan bool, 1),
	}
}

func (e *EventEncDec) processIngress() {
	for {
		select {
		case update := <-e.rpcIngress:
			go func() {
				for _, s := range update.GetSessions() {
					// Translate update to SyncJob
					// get all agent addresses
					agentAddresses := e.healthMgr.GetActiveNodesAddresses()
					msg := (*[4]byte)(unsafe.Pointer(&s.Label))[:]
					log.Println("New Label: ", msg)
					log.Println("Send Sync Jobs sequencer for agents: ", agentAddresses)
					// From centralized controller
					syncJob := core.NewSyncJob(s.SessionAddress, core.Sync, "", agentAddresses, msg, update.SequenceId, 0)
					// Send the new SyncJob to e.esEgress
					e.esEgress <- syncJob
				}
			}()

		case syncJobResult := <-e.esIngress:
			go func() {
				if syncJobResult.JobType == core.Sync {
					log.Println("Send Sync job to switch")
					// Translate syncJobResult to MdcPkt
					sessionAddressBytes := []byte(syncJobResult.SessionAddress)
					sessionAddress := binary.LittleEndian.Uint16(sessionAddressBytes)
					label := binary.LittleEndian.Uint32(syncJobResult.MsgContent)

					pktBytes, err := horus_net.CreateMdcPacket(horus_net.MdcTypeSyncState, 0, sessionAddress, label, syncJobResult.SequenceID)
					if err != nil {
						log.Println(err)
					} else {
						// send the new MdcSyncEvent to localSockEgress
						e.localSockEgress <- pktBytes
					}
					//log.Println(sessionAddress)
					//log.Println(finalMsg)
					//log.Println(pktBytes)
				} else if syncJobResult.JobType == core.Done {
					log.Println("Send Done job to controller")
					// Translate syncJobResult to MdcSyncEvent
					syncEvent := new(horus_pb.MdcSyncEvent)
					syncEvent.SessionAddresses = make([][]byte, 1)
					syncEvent.SessionAddresses[0] = syncJobResult.SessionAddress
					syncEvent.TorId = e.torId
					syncEvent.SequenceId = syncJobResult.SequenceID
					// send the new MdcSyncEvent to rpcEgress
					e.rpcEgress <- syncEvent
				}
			}()

		case activeNodeMsg := <-e.hmIngressActiveNode:
			log.Println("Send set-active-agent to switch")
			// Translate activeNodeMsg to MdcPkt
			nodeIDByte := byte(activeNodeMsg.ActiveNodeId)
			pktBytes, err := horus_net.CreateMdcPacket(horus_net.MdcTypeSetActiveAgent, nodeIDByte, 0, 0, 0)
			if err != nil {
				log.Println(err)
			} else {
				log.Println("Set Active Agent ", activeNodeMsg.ActiveNodeId)
				e.localSockEgress <- pktBytes
			}

		case dpMsg := <-e.localSockIngress:
			go func() {
				pkt := gopacket.NewPacket(dpMsg, layers.LayerTypeEthernet, gopacket.Default)
				if mdcLayer := pkt.Layer(horus_net.LayerTypeMdc); mdcLayer != nil {
					// Get actual Mdc data from this layer
					mdc, _ := mdcLayer.(*horus_net.Mdc)
					if mdc.Type == horus_net.MdcTypePing {
						// Ping-pong protocol
						nodeID := mdc.Agent
						e.healthMgr.OnNodePingRecv("", uint32(nodeID), 0)
						//log.Println("Ping from ", nodeID)
						// Pong pkt type
						// we need to modify one byte only, no need to re-serialize the pkt and create a new buffer!
						// Cons: if we modify the protocol, we need to change this!
						newPktBytes := pkt.Data()
						newPktBytes[14] = byte(horus_net.MdcTypePong)
						e.localSockEgress <- newPktBytes
					} else if mdc.Type == horus_net.MdcTypeSyncStateDone {
						// sync-state-done
						agentID := mdc.Agent //mdc.Payload[3]
						agentIDStr := strconv.Itoa(int(agentID))
						log.Println("sync-state-done:", agentID)

						addressBytes := make([]byte, 2)
						binary.LittleEndian.PutUint16(addressBytes, mdc.Address)

						// From agent
						syncJob := core.NewSyncJob(addressBytes, core.Done, agentIDStr,
							[]string{""}, nil, mdc.SequenceID, 0)
						// send it to eventSequencer
						e.esEgress <- syncJob
					}
				}
			}()

		default:
			continue
		}
	}
}

func (e *EventEncDec) Start() {
	go e.processIngress()
	<-e.doneChan
}
