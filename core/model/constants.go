package model

const MAX_VCLUSTER_WORKERS = 16 // Should match the hardcoded val in p4 code
const CPU_PORT_ID = 192         // Copy to CPU port in Tofino

// Unit used for avg. calculation based on #workers in rack
var WorkerQlenUnitMap = map[uint16]uint16{ // assuming 3bit showing fraction and 5bit decimal
	4:  8,
	8:  4,
	16: 2,
	32: 1,
}

// Used by leaf when forwarding Horus signal pkts to spine (key 100)
// or sending back replies (send to spine and then spine sends to client)
// var LeafUpstreamPortMap = map[uint16]uint16{
// 	0: 152, // 30/0
// 	1: 144, // 29/0
// 	2: 160, // 28/0
// 	3: 168, // 27/0
// }

// var UpstreamIDs = [3]int{100, 110, 120} // Spine ID 100 and client IDs 110, 120

// Used by spine when forwarding the task result to client machines
// var ClientPortMap = map[uint16]uint16{
// 	110: 56, // 9/0
// 	111: 58, //
// }
