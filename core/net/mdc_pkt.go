//go:build exclude
// +build exclude

package net

import (
	"encoding/binary"
	"errors"
	"log"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

type MdcType byte

const (
	EthernetTypeMdc       layers.EthernetType = 0x88b5 //0x9999
	MdcTypeUnlabeled      MdcType             = 0x01
	MdcTypeLabeled        MdcType             = 0x02
	MdcTypePing           MdcType             = 0xF0
	MdcTypePong           MdcType             = 0xF1
	MdcTypeSyncState      MdcType             = 0xF2
	MdcTypeSyncStateDone  MdcType             = 0xF3
	MdcTypeSetActiveAgent MdcType             = 0xF4
)

var lotsOfZeros [1024]byte

// Mdc Create custom layer structure
type Mdc struct {
	layers.BaseLayer

	Type       MdcType
	Agent      byte
	Address    uint16
	Label      uint32
	SequenceID uint32
}

// Register the layer type so we can use it
// The first argument is an ID. Use negative
// or 2000+ for custom layers. It must be unique
var LayerTypeMdc = gopacket.RegisterLayerType(
	2001,
	gopacket.LayerTypeMetadata{
		Name:    "Mdc",
		Decoder: gopacket.DecodeFunc(decodeMdc),
	},
)

func InitHorusDefinitions() {
	layers.EthernetTypeMetadata[EthernetTypeMdc] = layers.EnumMetadata{
		DecodeWith: gopacket.DecodeFunc(decodeMdc),
		Name:       "Horus",
		LayerType:  LayerTypeMdc,
	}
}

// When we inquire about the type, what type of layer should
// we say it is? We want it to return our custom layer type
func (mdc *Mdc) LayerType() gopacket.LayerType {
	return LayerTypeMdc
}

// LayerContents returns the information that our layer
// provides. In this case it is a header layer so
// we return the header information
func (mdc *Mdc) LayerContents() []byte {
	address := make([]byte, 2)
	binary.LittleEndian.PutUint16(address, uint16(mdc.Address))
	label := make([]byte, 4)
	binary.LittleEndian.PutUint32(label, uint32(mdc.Label))
	sequenceID := make([]byte, 4)
	binary.LittleEndian.PutUint32(sequenceID, uint32(mdc.SequenceID))

	mdcType := []byte{byte(mdc.Type)}
	finalMsg := append(mdcType, mdc.Agent)
	finalMsg = append(finalMsg, address...)
	finalMsg = append(finalMsg, label...)
	finalMsg = append(finalMsg, sequenceID...)
	return finalMsg
}

// LayerPayload returns the subsequent layer built
// on top of our layer or raw payload
func (mdc *Mdc) LayerPayload() []byte {
	return mdc.Payload
}

// SerializeTo writes the serialized form of this layer into the
// SerializationBuffer, implementing gopacket.SerializableLayer.
// See the docs for gopacket.SerializableLayer for more info.
func (mdc *Mdc) SerializeTo(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {
	//payload := b.Bytes()
	bytes, err := b.PrependBytes(12)
	if err != nil {
		return err
	}
	copy(bytes, []byte{byte(mdc.Type)})
	copy(bytes[1:], []byte{mdc.Agent})
	binary.BigEndian.PutUint16(bytes[2:], mdc.Address)
	binary.BigEndian.PutUint32(bytes[4:], mdc.Label)
	binary.BigEndian.PutUint32(bytes[8:], mdc.SequenceID)

	length := len(b.Bytes())
	if length < 50 {
		// Pad out to 50 bytes.
		padding, err := b.AppendBytes(50 - length)
		if err != nil {
			return err
		}
		copy(padding, lotsOfZeros[:])
	}
	return nil
}

func (mdc *Mdc) CanDecode() gopacket.LayerClass {
	return LayerTypeMdc
}

func (mdc *Mdc) NextLayerType() gopacket.LayerType {
	return gopacket.LayerTypePayload
}

// Custom decode function. We can name it whatever we want
// but it should have the same arguments and return value
// When the layer is registered we tell it to use this decode function
func (mdc *Mdc) DecodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	if len(data) < 12 {
		return errors.New("mdc packet too small")
	}
	mdc.Type = MdcType(data[0])
	mdc.Agent = data[1]
	mdc.Address = binary.BigEndian.Uint16(data[2:4])
	mdc.Label = binary.BigEndian.Uint32(data[4:8])
	mdc.SequenceID = binary.BigEndian.Uint32(data[8:12])
	mdc.BaseLayer = layers.BaseLayer{Contents: data[:12], Payload: data[12:]}
	return nil
}

func decodeMdc(data []byte, p gopacket.PacketBuilder) error {
	mdc := &Mdc{}
	err := mdc.DecodeFromBytes(data, p)
	if err != nil {
		return err
	}
	p.AddLayer(mdc)
	return p.NextDecoder(gopacket.LayerTypePayload)
}

func CreateMdcPacket(pktType MdcType, agent byte, address uint16, label uint32, sequenceID uint32) ([]byte, error) {
	etherType := EthernetTypeMdc

	srcMacAddr, err := net.ParseMAC("a0:b0:c0:d0:e0:f0")
	if err != nil {
		log.Println(err)
	}
	dstMacAddr, err := net.ParseMAC("a1:b1:c1:d1:e1:f1")
	if err != nil {
		log.Println(err)
	}

	eth := &layers.Ethernet{
		SrcMAC:       srcMacAddr,
		DstMAC:       dstMacAddr,
		EthernetType: etherType,
	}

	mdc := &Mdc{Type: pktType, Agent: agent, Address: address, Label: label, SequenceID: sequenceID}

	var pktLayers []gopacket.SerializableLayer
	pktLayers = append(pktLayers, eth, mdc)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	err = gopacket.SerializeLayers(buf, opts, pktLayers...)
	if err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func TestMdcPkt() {
	InitHorusDefinitions()

	if pktBytes, err := CreateMdcPacket(MdcTypePing, 0x10, 8, 512, 1); err != nil {
		log.Println(err)
	} else {
		log.Println(pktBytes)
		log.Println(len(pktBytes))

		pkt := gopacket.NewPacket(pktBytes, layers.LayerTypeEthernet, gopacket.Default)
		if mdcLayer := pkt.Layer(LayerTypeMdc); mdcLayer != nil {
			// Get actual Mdc data from this layer
			mdc, _ := mdcLayer.(*Mdc)
			log.Println(mdc.Type)
			log.Println(mdc.Agent)
			log.Println(mdc.Address)
			log.Println(mdc.Label)
			log.Println(mdc.SequenceID)

			newPktBytes := pkt.Data()
			newPktBytes[14] = byte(MdcTypePong)
			log.Println(newPktBytes)
			// Modify the pkt
			//mdc.Type = MdcTypePong
			//newBuffer := gopacket.NewSerializeBuffer()
			//opts := gopacket.SerializeOptions{}
			//err := gopacket.SerializePacket(newBuffer, opts, pkt)
			//if err != nil {
			//	panic(err)
			//}
		}
	}
}
