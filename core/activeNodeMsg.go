package core

type ActiveNodeMsg struct {
	ActiveNodeId uint32
}

func NewActiveNodeMsg(nodeId uint32) *ActiveNodeMsg {
	return &ActiveNodeMsg{ActiveNodeId: nodeId}
}
