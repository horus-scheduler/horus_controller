package mc_algorithm

import (
	"log"
	"sync"

	"github.com/khaledmdiab/horus_controller/core/model"
)

type SimpleMCAlgorithm struct {
	sessionTreeMapLock sync.RWMutex
	sessionTreeMap     map[string]*model.MulticastTree
	topology           *model.SimpleTopology
}

func NewSimpleMCAlgorithm(topology *model.SimpleTopology) *SimpleMCAlgorithm {
	return &SimpleMCAlgorithm{topology: topology,
		sessionTreeMap: make(map[string]*model.MulticastTree),
	}
}

func (a *SimpleMCAlgorithm) getTree(session *model.Session) *model.MulticastTree {
	var tree *model.MulticastTree
	if t, found := a.sessionTreeMap[session.SessionAddressString]; found {
		tree = t
	} else {
		tree = model.NewMulticastTree()
		a.sessionTreeMap[session.SessionAddressString] = tree
	}
	return tree
}

func (a *SimpleMCAlgorithm) OnSessionActivated(session *model.Session) *model.MulticastTree {
	return a.getTree(session)
}

func (a *SimpleMCAlgorithm) OnSessionDeactivated(session *model.Session) *model.MulticastTree {
	return a.getTree(session)
}

func (a *SimpleMCAlgorithm) OnSessionCreated(session *model.Session) *model.MulticastTree {
	a.sessionTreeMapLock.Lock()
	defer a.sessionTreeMapLock.Unlock()

	t := a.getTree(session)

	// Hosts
	t.SourceNode = a.topology.GetNode(session.Sender.Id, model.HostNode)
	t.UpstreamTorNode = t.SourceNode.Parents[0]

	log.Println("New Session, Multicast tree:")
	log.Println("Host", t.SourceNode.Id)
	log.Println("ToR", t.UpstreamTorNode.Address, t.UpstreamTorNode.Id)

	return t
}

func (a *SimpleMCAlgorithm) OnReceiverAdded(session *model.Session, receiver *model.Node) *model.MulticastTree {
	a.sessionTreeMapLock.Lock()
	defer a.sessionTreeMapLock.Unlock()

	t := a.getTree(session)

	// In this SimpleAlgorithm, we need to include the aggregate switch iff a non-Source ToR is included in the tree
	torNode := receiver.Parents[0]
	t.Lock()
	t.ReceiverNodes[receiver.Id] = receiver
	t.DownstreamTorNodes[torNode.Id] = torNode
	if torNode.Id != t.UpstreamTorNode.Id {
		aggNode := torNode.Parents[0]
		t.UpstreamAggNode = aggNode
		t.DownstreamAggNodes[aggNode.Id] = aggNode
	}
	t.Unlock()

	log.Println("New Receiver:")
	log.Println("Receivers:")
	for _, r := range t.ReceiverNodes {
		log.Println("Receiver ID: ", r.Id)
	}
	log.Println("Downstream ToRs:")
	for _, r := range t.DownstreamTorNodes {
		log.Println("ToR ID: ", r.Id)
	}
	log.Println("Downstream Agg:")
	for _, r := range t.DownstreamAggNodes {
		log.Println("Agg ID: ", r.Id)
	}

	return t
}

func (a *SimpleMCAlgorithm) OnReceiverRemoved(session *model.Session, receiver *model.Node) *model.MulticastTree {
	return a.getTree(session)
}

func (a *SimpleMCAlgorithm) OnReceiverRemovedByAddress(session *model.Session, receiverAddress string) *model.MulticastTree {
	return a.getTree(session)
}
