package ctrl_sw

import (
	"github.com/khaledmdiab/horus_controller/core"
	"github.com/khaledmdiab/horus_controller/core/net"
	horus_pb "github.com/khaledmdiab/horus_controller/protobuf"
	"github.com/sirupsen/logrus"

	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
)

type controller struct {
	Index   uint16
	ID      uint16
	Address string
	cfg     *rootConfig

	// Controller components
	bfrt *bfrtC.Client // BfRt client

	// Communicating with the ASIC
	asicIngress chan []byte // recv-from the ASIC
	asicEgress  chan []byte // send-to the ASIC
}

// Leaf-specific logic
type leafController struct {
	*controller

	// Components
	evEncDec    *LeafEventEncDec        // Main leaf logic
	rpcEndPoint *net.LeafRpcEndpoint    // RPC server (Horus messages)
	healthMgr   *core.NodeHealthManager // Tracking the health of downstream nodes
}

// Spine-specific logic
type spineController struct {
	*controller

	// Components
	evEncDec    *SpineEventEncDec       // Main spine logic
	rpcEndPoint *net.SpineRpcEndpoint   // RPC server (Horus messages)
	healthMgr   *core.NodeHealthManager // Tracking the health of downstream nodes
}

// TODO: create (for leaf/spine):
// 1. Health manager
// 2. Event Encoder/Decoder

func NewLeafController(index uint16, cfg *rootConfig) *leafController {
	asicEgress := make(chan []byte, net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, net.DefaultUnixSockRecvSize)
	activeNode := make(chan *core.HealthManagerMsg, net.DefaultUnixSockRecvSize)
	rpcIngress := make(chan *horus_pb.HorusMessage, net.DefaultRpcRecvSize)
	rpcEgress := make(chan *horus_pb.HorusMessage, net.DefaultRpcSendSize)

	// TODO: Leaf Health manager
	healthMgr := core.NewNodeHealthManager(activeNode, 1000)
	// TODO: Leaf RPC end point
	// rpcEndPoint := net.NewLeafRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)
	// TODO: object model

	ch := NewLeafEventEncDecChan(activeNode, rpcIngress, rpcEgress,
		asicIngress, asicEgress)

	evEncDec := NewLeafEventEncDec(ch, healthMgr)
	return &leafController{
		rpcEndPoint: nil,
		healthMgr:   healthMgr,
		evEncDec:    evEncDec,
		controller: &controller{
			Index: index,
			// ID:          ctrl.ID,
			// Address:     ctrl.Address,
			cfg:         cfg,
			asicIngress: asicIngress,
			asicEgress:  asicEgress,
		},
	}
}

func NewSpineController(index uint16, cfg *rootConfig) *spineController {
	asicEgress := make(chan []byte, net.DefaultUnixSockSendSize)
	asicIngress := make(chan []byte, net.DefaultUnixSockRecvSize)
	activeNode := make(chan *core.HealthManagerMsg, net.DefaultUnixSockRecvSize)
	rpcIngress := make(chan *horus_pb.HorusMessage, net.DefaultRpcRecvSize)
	rpcEgress := make(chan *horus_pb.HorusMessage, net.DefaultRpcSendSize)

	// TODO: Spine Health manager
	healthMgr := core.NewNodeHealthManager(activeNode, 1000)
	// TODO: Spine RPC end point
	// rpcEndPoint := net.NewSpineRpcEndpoint(cfg.LocalRpcAddress, cfg.RemoteRpcAddress, rpcIngressChan, rpcEgressChan)
	// TODO: object model

	ch := NewSpineEventEncDecChan(activeNode, rpcIngress, rpcEgress,
		asicIngress, asicEgress)

	evEncDec := NewSpineEventEncDec(ch, healthMgr)
	return &spineController{
		rpcEndPoint: nil,
		healthMgr:   nil,
		evEncDec:    evEncDec,
		controller: &controller{
			Index: index,
			// ID:          ctrl.ID,
			// Address:     ctrl.Address,
			cfg:         cfg,
			asicIngress: asicIngress,
			asicEgress:  asicEgress,
		},
	}
}

// Common controller init. logic goes here
func (c *controller) Start() {
	logrus.
		WithFields(logrus.Fields{"index": c.Index}).
		Infof("Starting the switch controller")
	target := bfrtC.NewTarget(bfrtC.WithDeviceId(c.cfg.DeviceID), bfrtC.WithPipeId(c.cfg.PipeID))
	c.bfrt = bfrtC.NewClient(c.cfg.BfrtAddress, c.cfg.P4Name, uint32(c.Index), target)
}

func (c *leafController) Start() {
	c.controller.Start()
	go c.healthMgr.Start()
	go c.evEncDec.Start()
}

func (c *spineController) Start() {
	c.controller.Start()
	// go c.healthMgr.Start()
	// go c.evEncDec.Start()
}
