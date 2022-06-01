package ctrl_sw

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	horus_net "github.com/khaledmdiab/horus_controller/core/net"
	"github.com/sirupsen/logrus"
)

type asicEndPoint struct {
	ifName string
	client *horus_net.RawSockClient

	// Leaf controllers channels
	leafIngress map[uint16]chan []byte
	leafEgress  map[uint16]chan []byte

	// Spine controllers channels
	// TODO: spine controllers *currently* don't talk with the ASIC
	spineIngress map[uint16]chan []byte
	spineEgress  map[uint16]chan []byte

	// asic channels
	asicIngress chan []byte
	asicEgress  chan []byte

	doneChan chan bool
}

func NewAsicEndPoint(ifName string,
	leaves []*leafController,
	spines []*spineController,
	asicIngress chan []byte,
	asicEgress chan []byte) *asicEndPoint {

	client := horus_net.NewRawSockClient(ifName, asicEgress, asicIngress)

	leafIngress := make(map[uint16]chan []byte)
	leafEgress := make(map[uint16]chan []byte)
	spineIngress := make(map[uint16]chan []byte)
	spineEgress := make(map[uint16]chan []byte)

	// TODO: if we plan to update leaves (add/remove) during run-time,
	// we need to update these channels as well
	for _, l := range leaves {
		leafIngress[l.ID] = l.asicIngress
		leafEgress[l.ID] = l.asicEgress
	}
	for _, s := range spines {
		spineIngress[s.ID] = s.asicIngress
		spineEgress[s.ID] = s.asicEgress
	}

	return &asicEndPoint{ifName: ifName,
		client:       client,
		asicIngress:  asicIngress,
		asicEgress:   asicEgress,
		leafIngress:  leafIngress,
		leafEgress:   leafEgress,
		spineIngress: spineIngress,
		spineEgress:  spineEgress,
		doneChan:     make(chan bool, 1),
	}
}

func (a *asicEndPoint) Start() {
	go a.read()
	go a.write()
	<-a.doneChan
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
				if ch, found := a.leafIngress[dstID]; found {
					ch <- pkt.Data()
				} else {
					logrus.Warn("Leaf controller ", dstID, " does not exist!")
				}
			}
		}
	}
}

// Writes pkts to the ASIC
func (a *asicEndPoint) write() {
	agg := make(chan []byte)
	for _, ch := range a.leafEgress {
		go func(c chan []byte) {
			for msg := range c {
				agg <- msg
			}
		}(ch)
	}

	for _, ch := range a.spineEgress {
		go func(c chan []byte) {
			for msg := range c {
				agg <- msg
			}
		}(ch)
	}

	for {
		select {
		case msg := <-agg:
			a.asicEgress <- msg
		}
	}
}
