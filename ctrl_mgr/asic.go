package ctrl_mgr

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/khaledmdiab/horus_controller/core/model"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type asicEndPoint struct {
	ifName string
	client *horus_net.RawSockClient

	// Leaf controllers channels
	leafIngress *model.ByteChanMap
	leafEgress  *model.ByteChanMap

	// Spine controllers channels
	// TODO: spine controllers *currently* don't talk with the ASIC
	spineIngress *model.ByteChanMap
	spineEgress  *model.ByteChanMap

	// asic channels
	asicIngress chan []byte
	asicEgress  chan []byte
	aggEgress   chan []byte

	doneChan chan bool
}

func NewAsicEndPoint(ifName string,
	leaves []*leafController,
	spines []*spineController,
	asicIngress chan []byte,
	asicEgress chan []byte) *asicEndPoint {

	client := horus_net.NewRawSockClient(ifName, asicEgress, asicIngress)

	leafIngress := model.NewByteChanMap()
	leafEgress := model.NewByteChanMap()
	spineIngress := model.NewByteChanMap()
	spineEgress := model.NewByteChanMap()
	aggEgress := make(chan []byte)

	a := &asicEndPoint{ifName: ifName,
		client:       client,
		asicIngress:  asicIngress,
		asicEgress:   asicEgress,
		aggEgress:    aggEgress,
		leafIngress:  leafIngress,
		leafEgress:   leafEgress,
		spineIngress: spineIngress,
		spineEgress:  spineEgress,
		doneChan:     make(chan bool, 1),
	}

	for _, l := range leaves {
		a.AddLeafCtrl(l)
	}
	for _, s := range spines {
		spineIngress.Store(s.ID, s.asicIngress)
		spineEgress.Store(s.ID, s.asicEgress)
	}
	return a
}

func (a *asicEndPoint) Start() {
	go a.read()
	go a.write()
	<-a.doneChan
}

func (a *asicEndPoint) AddLeafCtrl(leaf *leafController) {
	logrus.Infof("[ASIC] Adding a leaf controller %d", leaf.ID)
	a.leafIngress.Store(leaf.ID, leaf.asicIngress)
	a.leafEgress.Store(leaf.ID, leaf.asicEgress)
	go func(c chan []byte) {
		for msg := range c {
			a.aggEgress <- msg
		}
	}(leaf.asicEgress)
}

func (a *asicEndPoint) RemoveLeafCtrl(leaf *leafController) {
	logrus.Infof("[ASIC] Removing a leaf controller %d", leaf.ID)
	logrus.Warn("[ASIC] RemoveLeafCtrl is experimental")
	a.leafIngress.Delete(leaf.ID)
	a.leafEgress.Delete(leaf.ID)
}

// Reads pkts from the ASIC
func (a *asicEndPoint) read() {
	for {
		select {
		case msg := <-a.asicIngress:
			pkt := gopacket.NewPacket(msg, layers.LayerTypeEthernet, gopacket.Default)
			if horusLayer := pkt.Layer(horus_net.LayerTypeHorus); horusLayer != nil {
				horus, _ := horusLayer.(*horus_net.HorusPacket)
				dstID := horus.DstID
				ch, found := a.leafIngress.Load(dstID)
				if found {
					ch <- pkt.Data()
				} else {
					logrus.Warnf("[ASIC] Leaf controller %d does not exist", dstID)
				}
			}
		}
	}
}

// Writes pkts to the ASIC
func (a *asicEndPoint) write() {
	// for _, ch := range a.leafEgress.Internal() {
	// 	go func(c chan []byte) {
	// 		for msg := range c {
	// 			a.AggEgress <- msg
	// 		}
	// 	}(ch)
	// }

	// for _, ch := range a.spineEgress.Internal() {
	// 	go func(c chan []byte) {
	// 		for msg := range c {
	// 			a.AggEgress <- msg
	// 		}
	// 	}(ch)
	// }

	for {
		select {
		case msg := <-a.aggEgress:
			a.asicEgress <- msg
		}
	}
}
