package net

import (
	"encoding/binary"
	"errors"
	"log"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

const (
	// Default port
	HORUS_UDP_PORT layers.UDPPort = 1234

	// Pkt types
	PKT_TYPE_KEEP_ALIVE    byte = 0x0B
	PKT_TYPE_WORKER_ID     byte = 0x0C
	PKT_TYPE_WORKER_ID_ACK byte = 0x0D

	// Dst types
	DST_TYPE_LEAF  uint16 = 0x0001
	DST_TYPE_SPINE uint16 = 0x0002
)

var lotsOfZeros [1024]byte

// Horus Create custom layer structure
type HorusPacket struct {
	layers.BaseLayer

	PktType   byte
	ClusterID uint16
	SrcID     uint16
	DstID     uint16
	QLen      uint16
	SeqNum    uint16
	// fields below are sample app layer headers: not needed for task scheduling and not parsed by switches
	RestOfData []byte
}

/*
type HorusPayload struct {
	client_id   uint16
	req_id      uint32
	pkts_length uint32
	runNs       uint64
	genNs       uint64
	app_data    [16]uint64
}
*/

// Register the layer type so we can use it
// The first argument is an ID. Use negative
// or 2000+ for custom layers. It must be unique
var LayerTypeHorus = gopacket.RegisterLayerType(
	2001,
	gopacket.LayerTypeMetadata{
		Name:    "Horus",
		Decoder: gopacket.DecodeFunc(decodeHorus),
	},
)

func InitHorusDefinitions() {
	layers.EthernetTypeMetadata[LayerTypeHorus] = layers.EnumMetadata{
		DecodeWith: gopacket.DecodeFunc(decodeHorus),
		Name:       "Horus",
		LayerType:  LayerTypeHorus,
	}
}

// When we inquire about the type, what type of layer should
// we say it is? We want it to return our custom layer type
func (horus *HorusPacket) LayerType() gopacket.LayerType {
	return LayerTypeHorus
}

// LayerPayload returns the subsequent layer built
// on top of our layer or raw payload
func (horus *HorusPacket) LayerPayload() []byte {
	return horus.RestOfData
}

func (horus *HorusPacket) CanDecode() gopacket.LayerClass {
	return LayerTypeHorus
}

func (horus *HorusPacket) NextLayerType() gopacket.LayerType {
	return gopacket.LayerTypePayload
}

// SerializeTo writes the serialized form of this layer into the
// SerializationBuffer, implementing gopacket.SerializableLayer.
// See the docs for gopacket.SerializableLayer for more info.
func (horus *HorusPacket) SerializeTo(b gopacket.SerializeBuffer, opts gopacket.SerializeOptions) error {
	//payload := b.Bytes()
	bytes, err := b.PrependBytes(11)
	if err != nil {
		return err
	}
	copy(bytes, []byte{byte(horus.PktType)})
	binary.BigEndian.PutUint16(bytes[1:], horus.ClusterID)
	binary.BigEndian.PutUint16(bytes[3:], horus.SrcID)
	binary.BigEndian.PutUint16(bytes[5:], horus.DstID)
	binary.BigEndian.PutUint16(bytes[7:], horus.QLen)
	binary.BigEndian.PutUint16(bytes[9:], horus.SeqNum)

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

// Custom decode function. We can name it whatever we want
// but it should have the same arguments and return value
// When the layer is registered we tell it to use this decode function
func (horus *HorusPacket) DecodeFromBytes(data []byte, df gopacket.DecodeFeedback) error {
	if len(data) < 12 {
		return errors.New("horus_hdr packet too small")
	}
	horus.PktType = byte(data[0])
	horus.ClusterID = binary.BigEndian.Uint16(data[1:3])
	horus.SrcID = binary.BigEndian.Uint16(data[3:5])
	horus.DstID = binary.BigEndian.Uint16(data[5:7])
	horus.QLen = binary.BigEndian.Uint16(data[7:9])
	horus.SeqNum = binary.BigEndian.Uint16(data[9:11])

	horus.BaseLayer = layers.BaseLayer{Contents: data[:11], Payload: data[12:]}
	return nil
}

func decodeHorus(data []byte, p gopacket.PacketBuilder) error {
	horus := &HorusPacket{}
	err := horus.DecodeFromBytes(data, p)
	if err != nil {
		return err
	}
	p.AddLayer(horus)
	return p.NextDecoder(gopacket.LayerTypePayload)
}

func CreateFullHorusPacket(horus *HorusPacket,
	srcIP net.IP,
	dstIP net.IP) ([]byte, error) {
	srcMacAddr, err := net.ParseMAC("00:02:00:00:03:00")
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
		EthernetType: layers.EthernetTypeIPv4,
	}

	ip := &layers.IPv4{
		Version:  4,
		TTL:      64,
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Protocol: layers.IPProtocolUDP,
	}

	udp := &layers.UDP{
		SrcPort: HORUS_UDP_PORT,
		DstPort: HORUS_UDP_PORT,
	}

	var pktLayers []gopacket.SerializableLayer
	pktLayers = append(pktLayers, eth, ip, udp, horus)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: false,
		FixLengths:       false,
	}
	err = gopacket.SerializeLayers(buf, opts, pktLayers...)
	if err != nil {
		return []byte{}, err
	}
	return buf.Bytes(), nil
}

func TestHorusPkt(pkt_type byte,
	cluster_id uint16,
	src_id uint16,
	dst_id uint16,
	seq_num uint16,
	payload []byte) {

	horusPkt := &HorusPacket{
		PktType:    pkt_type,
		ClusterID:  cluster_id,
		SrcID:      src_id,
		DstID:      dst_id,
		SeqNum:     seq_num,
		RestOfData: payload,
	}
	if pktBytes, err := CreateFullHorusPacket(horusPkt, net.IP{10, 1, 0, 1}, net.IP{10, 1, 0, 2}); err != nil {
		log.Println(err)
	} else {
		log.Println(pktBytes)
		log.Println(len(pktBytes))

		pkt := gopacket.NewPacket(pktBytes, layers.LayerTypeEthernet, gopacket.Default)
		if horusLayer := pkt.Layer(LayerTypeHorus); horusLayer != nil {
			// Get actual Mdc data from this layer
			horus, _ := horusLayer.(*HorusPacket)
			log.Println(horus.PktType)
			log.Println(horus.SrcID)
			log.Println(horus.DstID)

			newPktBytes := pkt.Data()
			newPktBytes[14] = byte(PKT_TYPE_WORKER_ID)
			log.Println(newPktBytes)
		}
	}
}
