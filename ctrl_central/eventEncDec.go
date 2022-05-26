//go:build exclude
// +build exclude

package ctrl_central

import (
	"encoding/binary"
	"log"
	"math"

	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/model"
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
	// send-to healthManager

	// gRPC channels
	// recv-from gRPC App server
	rpcAppIngress chan *horus_pb.MdcAppEvent
	// Not Used: send-to gRPC App client
	rpcAppEgress chan *horus_pb.MdcSyncEvent

	// recv-from gRPC ToR server
	rpcTorIngress chan *horus_pb.MdcSyncEvent
	// send-to gRPC ToR client
	rpcTorEgress chan *horus_pb.MdcSessionUpdateEvent
}

// NewEventEncDecChan ...
func NewEventEncDecChan(esIngress chan *core.SyncJobResult,
	esEgress chan *core.SyncJob,
	rpcAppIngress chan *horus_pb.MdcAppEvent,
	rpcAppEgress chan *horus_pb.MdcSyncEvent,
	rpcTorIngress chan *horus_pb.MdcSyncEvent,
	rpcTorEgress chan *horus_pb.MdcSessionUpdateEvent) *EventEncDecChan {
	return &EventEncDecChan{
		esEgress:      esEgress,
		esIngress:     esIngress,
		rpcAppIngress: rpcAppIngress,
		rpcAppEgress:  rpcAppEgress,
		rpcTorIngress: rpcTorIngress,
		rpcTorEgress:  rpcTorEgress,
	}
}

// EventEncDec ...
type EventEncDec struct {
	*EventEncDecChan
	topology   *model.SimpleTopology
	labeler    label.Labeler
	sessionMgr *core.SessionManager
	es         *sequencer.SimpleEventSequencer
	doneChan   chan bool
}

// NewEventEncDec ...
func NewEventEncDec(topology *model.SimpleTopology,
	labeler label.Labeler,
	encDecChan *EventEncDecChan,
	sessionMgr *core.SessionManager,
	eventSequencer *sequencer.SimpleEventSequencer) *EventEncDec {
	return &EventEncDec{
		EventEncDecChan: encDecChan,
		topology:        topology,
		labeler:         labeler,
		sessionMgr:      sessionMgr,
		es:              eventSequencer,
		doneChan:        make(chan bool, 1),
	}
}

func (e *EventEncDec) processIngress() {
	for {
		select {

		case appEvent := <-e.rpcAppIngress:
			go func() {
				eventType := appEvent.Type
				// Translate update to SyncJob
				sender := e.topology.GetNode(appEvent.HostId, model.HostNode)
				if eventType == horus_pb.MdcAppEvent_Create {
					session := model.NewSession(appEvent.SessionAddress, true, sender)
					e.sessionMgr.CreateSession(session)
				} else if eventType == horus_pb.MdcAppEvent_Join {
					sessionAddrStr := string(appEvent.SessionAddress)
					tree := e.sessionMgr.AddReceiver(sessionAddrStr, sender)
					sessionLabel := e.labeler.GetLabel(tree)
					log.Println("Session Label: ", sessionLabel)
					torAddresses := make([]string, 0)
					torAddresses = append(torAddresses, tree.UpstreamTorNode.Address)
					for _, dTor := range tree.DownstreamTorNodes {
						// ignore dTor.Address if it's already the upstream ToR
						if dTor.Address != torAddresses[0] {
							torAddresses = append(torAddresses, dTor.Address)
						}
					}
					syncJob := core.NewSyncJob(appEvent.SessionAddress, core.Sync,
						"", torAddresses, sessionLabel, math.MaxUint32, 0)
					// Send the syncJob to eventSequencer
					e.esEgress <- syncJob
				} else if eventType == horus_pb.MdcAppEvent_Leave {
					//sessionAddrStr := string(appEvent.SessionAddress)
					//e.sessionMgr.RemoveReceiver(sessionAddrStr, sender)
				}
			}()

		// Messages coming from eventSequencer
		case syncJobResult := <-e.esIngress:
			go func() {
				if syncJobResult.JobType == core.Sync {
					log.Println("Send sync job to ToRs")
					// Send this to all ToRs
					for _, receiverAddress := range syncJobResult.Receivers {
						receiverNode := e.topology.GetNodeByAddress(receiverAddress, model.TorNode)
						// Create MdcSessionUpdateEvent objects
						updateEvent := new(horus_pb.MdcSessionUpdateEvent)
						updateEvent.TorId = receiverNode.Id
						updateEvent.SequenceId = syncJobResult.SequenceID
						updateEvent.Sessions = make([]*horus_pb.MdcSessionUpdateEvent_SessionLabel, 1)
						updateEvent.Sessions[0] = &horus_pb.MdcSessionUpdateEvent_SessionLabel{}
						updateEvent.Sessions[0].SessionAddress = syncJobResult.SessionAddress
						updateEvent.Sessions[0].Label = binary.BigEndian.Uint32(syncJobResult.MsgContent)
						// send them to rpcTorEgress
						e.rpcTorEgress <- updateEvent
					}

				} else if syncJobResult.JobType == core.Done {
					// Ignore this message
					log.Println("Received Done message from sequencer")
				}
			}()

		case syncEvent := <-e.rpcTorIngress:
			go func() {
				log.Println("Received Done from ToR")
				// Translate syncJob to SyncJob
				torNode := e.topology.GetNode(syncEvent.TorId, model.TorNode)
				sessionAddr := syncEvent.SessionAddresses[0]
				// From leaf controller
				syncJob := core.NewSyncJob(sessionAddr, core.Done,
					torNode.Address, []string{""}, nil, syncEvent.SequenceId, 0)
				// Send it to eventSequencer
				e.esEgress <- syncJob
			}()

		default:
			continue
		}
	}
}

// Start ...
func (e *EventEncDec) Start() {
	go e.processIngress()
	<-e.doneChan
}
