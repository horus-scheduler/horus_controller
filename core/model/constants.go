package model

const MAX_VCLUSTER_WORKERS = 16 // Should match the hardcoded val in p4 code
const CPU_PORT_ID = 192         // Copy to CPU port in Tofino

// Unit used for avg. calculation based on #workers in rack
var WorkerQlenUnitMap = map[uint16]uint16{ // assuming 3bit showing fraction and 5bit decimal
	4:  8,
	8:  4,
	32: 1,
}

var UpstreamPortMap = map[uint16]uint16{
	100: 152,
	110: 144,
	111: 160,
}
