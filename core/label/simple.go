package label

import "github.com/khaledmdiab/horus_controller/core/model"

type SimpleLabelCalculator struct {
	topology         *model.SimpleTopology
	portLabelMapping map[int]byte
}

func NewLabelCalculator(topology *model.SimpleTopology) *SimpleLabelCalculator {
	portLabelMapping := make(map[int]byte)
	portLabelMapping[0] = 0x01
	portLabelMapping[1] = 0x02
	portLabelMapping[2] = 0x04
	portLabelMapping[3] = 0x08
	portLabelMapping[4] = 0x10
	portLabelMapping[5] = 0x20
	portLabelMapping[6] = 0x40
	portLabelMapping[7] = 0x80
	return &SimpleLabelCalculator{topology: topology, portLabelMapping: portLabelMapping}
}

func (l *SimpleLabelCalculator) GetLabel(tree *model.MulticastTree) []byte {
	label := make([]byte, 4)

	// ToR -> Aggregate (Upstream)
	label[0] = l.getDownstreamLabel(tree.UpstreamTorNode, tree.ReceiverNodes)
	if tree.UpstreamAggNode != nil {
		label[0] |= l.getUpstreamLabel(tree.UpstreamAggNode, tree.UpstreamTorNode)
	}

	// Aggregate -> ToR (Downstream)
	label[1] = 0x00
	if len(tree.DownstreamAggNodes) > 0 {
		label[1] = l.getDownstreamLabel(tree.DownstreamAggNodes[0], tree.DownstreamTorNodes)
	}

	label[2] = 0x00
	// ToR -> Host
	label[3] = 0x00

	return label
}

func (l *SimpleLabelCalculator) getDownstreamLabel(upstreamNode *model.Node,
	downstreamNodes map[uint32]*model.Node) byte {
	uId := upstreamNode.Id
	label := byte(0x00)
	for _, dNode := range downstreamNodes {
		if dNode.Id/uint32(l.topology.TorCount) == uId {
			portId := int(dNode.Id - uId*uint32(l.topology.TorCount))
			label = label | l.portLabelMapping[portId]
		}
	}
	return label
}

func (l *SimpleLabelCalculator) getUpstreamLabel(upstreamNode *model.Node,
	downstreamNode *model.Node) byte {
	uId := upstreamNode.Id
	dId := downstreamNode.Id
	label := byte(0x00)
	if dId/uint32(l.topology.TorCount) == uId {
		portId := int(dId - uId*uint32(l.topology.TorCount))
		label = label | l.portLabelMapping[l.topology.TorCount+portId]
	}

	return label
}
