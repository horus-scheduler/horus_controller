package model

import (
	"errors"
	"fmt"
	"strconv"
	"sync"

	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
)

type Topology struct {
	sync.RWMutex
	Root         *Node
	Clients      *NodeMap
	Servers      *NodeMap
	Leaves       *NodeMap
	Spines       *NodeMap
	Cores        *NodeMap
	AsicRegistry *AsicRegistry
}

func (s *Topology) FindPort(asicStr string, portSpecStr string) (*Port, error) {
	asic, asicFound := s.AsicRegistry.AsicMap.Load(asicStr)
	if asicFound {
		port, portFound := asic.PortRegistry.PortMap.Load(portSpecStr)
		if portFound {
			return port, nil
		}
		return nil, fmt.Errorf("ASIC %s has no port %s", asicStr, portSpecStr)
	}
	return nil, fmt.Errorf("ASIC %s doesn't exist", asicStr)
}

func NewDCNFromConf(topoRootCfg *topoRootConfig) *Topology {
	topoCfg := topoRootCfg.TopoConf
	asicConfigs := topoRootCfg.AsicConf.Asics
	portConfigs := topoRootCfg.PortConf.PortConfig
	portGroups := topoRootCfg.PortConf.PortGroup
	s := &Topology{
		Servers:      NewNodeMap(),
		Leaves:       NewNodeMap(),
		Spines:       NewNodeMap(),
		Cores:        NewNodeMap(),
		Clients:      NewNodeMap(),
		AsicRegistry: NewAsicRegistry(asicConfigs, portConfigs, portGroups),
	}

	for _, clientConf := range topoCfg.Clients {
		if port, err := s.FindPort(clientConf.Asic, clientConf.Port); err == nil {
			client := NewClient(clientConf.ID, port)
			s.Clients.Store(client.ID, client)
		} else {
			logrus.Fatalf("client %d has no port: %s", clientConf.ID, err.Error())
		}
	}

	s.Root = NewNode("", "", 0, 0, NodeType_Core)
	s.Root.Parent = nil
	s.Cores.Store(s.Root.ID, s.Root)

	for _, spineConf := range topoCfg.Spines {
		spine := NewSpine(spineConf.Address, spineConf.ID)
		spine.Parent = s.Root
		s.Root.Children = append(s.Root.Children, spine)
		s.Spines.Store(spine.ID, spine)

		for _, leafID := range spineConf.LeafIDs {
			// workerIDs are local per leaf
			var workerID uint16 = 0
			leafConf := topoCfg.Leaves[leafID]
			// Parham modified the line below pass PortID for leaf
			// leaf := NewNode(leafConf.Address, leafConf.MgmtAddress, leafConf.ID, leafConf.PortID, NodeType_Leaf)
			dsPort, err1 := s.FindPort(leafConf.Asic, leafConf.DsPort)
			usPort, err2 := s.FindPort(leafConf.Asic, leafConf.UsPort)
			if err1 != nil || err2 != nil {
				logrus.Fatal(err1, err2)
			}
			leaf := NewLeaf(leafConf.Address, leafConf.MgmtAddress, leafConf.ID, dsPort, usPort)
			leaf.Index = leafConf.Index
			leaf.Parent = spine
			spine.Children = append(spine.Children, leaf)
			leaf.FirstWorkerID = workerID
			for _, serverID := range leafConf.ServerIDs {
				serverConf := topoCfg.Servers[serverID]
				// server := NewNode(serverConf.Address, "", serverConf.ID, serverConf.PortID, NodeType_Server)
				port, err := s.FindPort(leafConf.Asic, serverConf.Port)
				if err != nil {
					logrus.Fatal(err)
				}
				server := NewServer(serverConf.Address, serverConf.ID, port)
				server.Parent = leaf
				leaf.Children = append(leaf.Children, server)
				// Set worker IDs per Server
				// worker IDs are contigous for Servers belonging to the same Leaf
				server.FirstWorkerID = workerID
				workerID += serverConf.WorkersCount
				// if workerID == 0 -> server.LastWorkerID = 0
				// When server.FirstWorkerID == server.LastWorkerID = 0 -> this server has no workers
				server.LastWorkerID = workerID - 1

				s.Servers.Store(server.ID, server)
			}
			leaf.LastWorkerID = workerID - 1
			s.Leaves.Store(leaf.ID, leaf)
		}
	}

	return s
}

func NewDCNFromTopoInfo(topoInfo *horus_pb.TopoInfo) *Topology {
	s := &Topology{
		Servers: NewNodeMap(),
		Leaves:  NewNodeMap(),
		Spines:  NewNodeMap(),
		Cores:   NewNodeMap(),
		Clients: NewNodeMap(),
	}
	// Implement the AsicRegistry
	s.Root = NewNode("", "", 0, 0, NodeType_Core)
	s.Root.Parent = nil
	s.Cores.Store(s.Root.ID, s.Root)

	for _, spineInfo := range topoInfo.Spines {
		spine := NewNode(spineInfo.Address, "", uint16(spineInfo.Id), 0, NodeType_Spine)
		spine.Parent = s.Root
		s.Root.Children = append(s.Root.Children, spine)
		s.Spines.Store(spine.ID, spine)

		for _, leafInfo := range spineInfo.Leaves {
			// workerIDs are local per leaf
			var workerID uint16 = 0
			// Parham: Modified the line below, passing PortId
			leaf := NewNode(leafInfo.Address, leafInfo.MgmtAddress, uint16(leafInfo.Id), uint16(leafInfo.PortId), NodeType_Leaf)
			leaf.Index = uint16(leafInfo.Index)
			leaf.Parent = spine
			spine.Children = append(spine.Children, leaf)
			leaf.FirstWorkerID = workerID
			for _, serverInfo := range leafInfo.Servers {
				server := NewNode(serverInfo.Address, "",
					uint16(serverInfo.Id),
					uint16(serverInfo.PortId),
					NodeType_Server)
				server.Parent = leaf
				leaf.Children = append(leaf.Children, server)
				// Set worker IDs per Server
				// worker IDs are contigous for Servers belonging to the same Leaf
				server.FirstWorkerID = workerID
				workerID += uint16(serverInfo.WorkersCount)
				// if workerID == 0 -> server.LastWorkerID = 0
				// When server.FirstWorkerID == server.LastWorkerID = 0 -> this server has no workers
				server.LastWorkerID = workerID - 1

				s.Servers.Store(server.ID, server)
			}
			leaf.LastWorkerID = workerID - 1
			s.Leaves.Store(leaf.ID, leaf)
		}
	}

	return s
}

func (s *Topology) EncodeToTopoInfo() *horus_pb.TopoInfo {
	topoInfo := &horus_pb.TopoInfo{}
	for _, spine := range s.Spines.Internal() {
		spineInfo := &horus_pb.SpineInfo{Id: uint32(spine.ID), Address: spine.Address}
		topoInfo.Spines = append(topoInfo.Spines, spineInfo)
		spine.RLock()
		for _, leaf := range spine.Children {
			leafInfo := &horus_pb.LeafInfo{}
			leaf.RLock()
			leafInfo.Id = uint32(leaf.ID)
			leafInfo.Index = uint32(leaf.Index)
			leafInfo.Address = leaf.Address
			leafInfo.PortId = uint32(leaf.PortId) // Parham: Added this line, please check correctness
			leafInfo.MgmtAddress = leaf.MgmtAddress
			leafInfo.SpineID = uint32(spine.ID)
			for _, server := range leaf.Children {
				serverInfo := &horus_pb.ServerInfo{}
				server.RLock()
				serverInfo.Id = uint32(server.ID)
				serverInfo.Address = server.Address
				serverInfo.PortId = uint32(server.PortId)
				var workersCount uint16 = 0
				if server.LastWorkerID > server.FirstWorkerID {
					workersCount = server.LastWorkerID - server.FirstWorkerID + 1
				}
				serverInfo.WorkersCount = uint32(workersCount)
				server.RUnlock()
				leafInfo.Servers = append(leafInfo.Servers, serverInfo)
			}
			leaf.RUnlock()
			spineInfo.Leaves = append(spineInfo.Leaves, leafInfo)
		}
		spine.RUnlock()
	}
	return topoInfo
}

func (s *Topology) EncodeToTopoInfoAtLeaf(leafInfo *horus_pb.LeafInfo) *horus_pb.TopoInfo {
	topoInfo := &horus_pb.TopoInfo{}
	leaf := s.GetNode(uint16(leafInfo.Id), NodeType_Leaf)
	if leaf == nil {
		return nil
	}
	if leaf.Parent == nil {
		return nil
	}

	spine := s.GetNode(leaf.Parent.ID, NodeType_Spine)
	spineInfo := &horus_pb.SpineInfo{Id: uint32(spine.ID), Address: spine.Address}
	topoInfo.Spines = append(topoInfo.Spines, spineInfo)

	retLeafInfo := &horus_pb.LeafInfo{}

	spine.RLock()
	retLeafInfo.SpineID = uint32(spine.ID)
	spine.RUnlock()

	leaf.RLock()
	retLeafInfo.Id = uint32(leaf.ID)
	retLeafInfo.Index = uint32(leaf.Index)
	retLeafInfo.Address = leaf.Address
	retLeafInfo.PortId = uint32(leaf.PortId) // Parham: added this line, please check
	retLeafInfo.MgmtAddress = leaf.MgmtAddress
	for _, server := range leaf.Children {
		serverInfo := &horus_pb.ServerInfo{}
		server.RLock()
		serverInfo.Id = uint32(server.ID)
		serverInfo.Address = server.Address
		serverInfo.PortId = uint32(server.PortId)
		var workersCount uint16 = 0
		if server.LastWorkerID > server.FirstWorkerID {
			workersCount = server.LastWorkerID - server.FirstWorkerID + 1
		}
		serverInfo.WorkersCount = uint32(workersCount)
		server.RUnlock()
		retLeafInfo.Servers = append(retLeafInfo.Servers, serverInfo)
	}
	leaf.RUnlock()
	spineInfo.Leaves = append(spineInfo.Leaves, retLeafInfo)

	return topoInfo
}

func (s *Topology) ClearLeaves() {
	for _, spine := range s.Spines.Internal() {
		spine.Children = nil
	}

	s.Leaves = NewNodeMap()
	s.Servers = NewNodeMap()
}

func (s *Topology) GetNode(nodeId uint16, nodeType NodeType) *Node {
	var nodes *NodeMap
	if nodeType == NodeType_Server {
		nodes = s.Servers
	} else if nodeType == NodeType_Leaf {
		nodes = s.Leaves
	} else if nodeType == NodeType_Spine {
		nodes = s.Spines
	} else if nodeType == NodeType_Client {
		nodes = s.Clients
	}

	node, found := nodes.Load(nodeId)
	if found {
		return node
	}
	return nil
}

func (s *Topology) AddLeafToSpine(leafInfo *horus_pb.LeafInfo) (*Node, error) {
	spineID := uint16(leafInfo.SpineID)
	leafID := uint16(leafInfo.Id)
	spine := s.GetNode(spineID, NodeType_Spine)
	leaf := s.GetNode(leafID, NodeType_Leaf)
	// Sanity check
	// 1. Spine exists
	// 2. Leaf doesn't exist
	// 3. If mgmtAddress exists, ensures that no other leaf has the same RPC address
	if spine == nil {
		return nil, errors.New("spine " + strconv.Itoa(int(spineID)) + " doesn't exist")
	}
	if leaf != nil {
		if leaf.Parent == spine {
			logrus.Warnf("[Topology] Leaf %d already exists", leaf.ID)
			return nil, errors.New("leaf " + strconv.Itoa(int(leafID)) + " already exists")
		} else {
			return nil, errors.New("leaf " + strconv.Itoa(int(leafID)) + " already exists with mismatched spine")
		}
	}

	for _, otherLeaf := range spine.Children {
		if otherLeaf.MgmtAddress == leafInfo.MgmtAddress {
			if otherLeaf.Address == leafInfo.Address {
				return nil, errors.New("leaf address (" + leafInfo.Address + ") is already being used in the manager: " + leafInfo.MgmtAddress)
			}
		}
	}

	// All checks passed.
	// Parham: modified the line below, passing PortId from protofbuf to Node.
	leaf = NewNode(leafInfo.Address, leafInfo.MgmtAddress, leafID, uint16(leafInfo.PortId), NodeType_Leaf)
	leaf.Parent = spine
	spine.Lock()
	spine.Children = append(spine.Children, leaf)
	leaf.Index = uint16(len(spine.Children) - 1)
	spine.Unlock()
	leaf.FirstWorkerID = 0
	leaf.LastWorkerID = 0
	s.Leaves.Store(leaf.ID, leaf)

	return leaf, nil
}

func (s *Topology) AddServerToLeaf(serverInfo *horus_pb.ServerInfo, leafID uint16) (*Node, error) {
	leaf := s.GetNode(leafID, NodeType_Leaf)
	existingServer := s.GetNode(uint16(serverInfo.Id), NodeType_Server)
	if leaf == nil {
		return nil, errors.New("leaf " + strconv.Itoa(int(leafID)) + " doesn't exist")
	}
	if existingServer != nil {
		if existingServer.Parent == leaf {
			logrus.Warnf("[Topology] Server %d already exists", existingServer.ID)
			return nil, errors.New("server " + strconv.Itoa(int(serverInfo.Id)) + " already exists")
		} else {
			return nil, errors.New("server " + strconv.Itoa(int(serverInfo.Id)) + " already exists with mismatched leaf")
		}
	}
	leaf.Lock()
	defer leaf.Unlock()

	server := NewNode(serverInfo.Address, "", uint16(serverInfo.Id), uint16(serverInfo.PortId), NodeType_Server)
	server.Parent = leaf
	leaf.Children = append(leaf.Children, server)
	leaf.FirstWorkerID = 0
	var workerID uint16 = 0
	if leaf.LastWorkerID > 0 {
		workerID = leaf.LastWorkerID + 1
	}
	server.FirstWorkerID = workerID
	workerID += uint16(serverInfo.WorkersCount)
	server.LastWorkerID = workerID - 1
	leaf.LastWorkerID = server.LastWorkerID
	s.Servers.Store(server.ID, server)
	return server, nil
}

// Input: Global serverID
// Output: a list of updated servers and boolean indicating whether it's removed
func (s *Topology) RemoveServer(serverID uint16) (bool, []*Node) {
	logrus.Debugf("[Topology] Removing server %d", serverID)
	server := s.GetNode(serverID, NodeType_Server)
	var updatedServers []*Node
	if server == nil {
		logrus.Debugf("[Topology] Server %d isn't found", serverID)
		return false, updatedServers
	}
	s.Lock()
	defer s.Unlock()

	leaf := server.Parent
	var workerID uint16 = 0
	var removedIdx int = -1
	for sIdx, srv := range leaf.Children {
		if srv.ID != serverID {
			workersCount := srv.LastWorkerID - srv.FirstWorkerID + 1
			srv.FirstWorkerID = workerID
			workerID += workersCount
			srv.LastWorkerID = workerID - 1
			if removedIdx > -1 {
				updatedServers = append(updatedServers, srv)
			}
		} else {
			removedIdx = sIdx
		}
	}

	leaf.FirstWorkerID = 0
	if workerID == 0 {
		leaf.LastWorkerID = 0
	} else {
		leaf.LastWorkerID = workerID - 1
	}
	logrus.Debug("[Topology] Updating leaf's children")
	leaf.Lock()
	leaf.Children = append(leaf.Children[:removedIdx], leaf.Children[removedIdx+1:]...)
	leaf.Unlock()
	logrus.Debug("[Topology] Leaf's children are updated")
	s.Servers.Delete(serverID)
	logrus.Debugf("[Topology] Number of servers to be updated = %d", len(updatedServers))
	return true, updatedServers
}

// Return the local index of the removed leaf within the spine and the leaves to be updated
// Input: Global leafID
func (s *Topology) RemoveLeaf(leafID uint16) (int, []*Node) {
	var updated []*Node
	leaf := s.GetNode(leafID, NodeType_Leaf)

	if leaf == nil {
		return -1, updated
	}
	logrus.Debug("Removing leaf: ", leaf.ID)
	// Remove children *before* the lock
	for {
		if len(leaf.Children) == 0 {
			break
		}
		s.RemoveServer(leaf.Children[0].ID)
	}
	s.Lock()
	defer s.Unlock()

	spine := leaf.Parent
	var removedIdx int = 0
	include := false
	for lIdx, l := range spine.Children {
		if l.ID == leafID {
			removedIdx = lIdx
			include = true
		} else {
			if include {
				l.Index -= 1
				updated = append(updated, l)
			}
		}
	}
	spine.Children = append(spine.Children[:removedIdx], spine.Children[removedIdx+1:]...)
	s.Leaves.Delete(leafID)

	return removedIdx, updated
}

func (s *Topology) Debug() {
	logrus.Debug("Spines Count=", len(s.Spines.Internal()))
	logrus.Debug("Leaves Count=", len(s.Leaves.Internal()))
	logrus.Debug("Servers Count=", len(s.Servers.Internal()))

	// DFS
	stack := make([]*Node, 0)
	stack = append(stack, s.Root)
	var node *Node
	for {
		if len(stack) == 0 {
			break
		}
		node, stack = stack[len(stack)-1], stack[:len(stack)-1]
		node.RLock()
		if node.Type == NodeType_Spine {
			logrus.Debug("- Spine: ", node.ID)
		} else if node.Type == NodeType_Leaf {
			logrus.Debug("-- Leaf: ", node.ID,
				", Index: ", node.Index,
				", First WID: ", node.FirstWorkerID,
				", Last WID: ", node.LastWorkerID)
		} else if node.Type == NodeType_Server {
			logrus.Debug("--- Server: ", node.ID, ", First WID: ", node.FirstWorkerID, ", Last WID: ", node.LastWorkerID)
		}
		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, node.Children[i])
		}
		node.RUnlock()
	}
}
