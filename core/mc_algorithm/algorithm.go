package mc_algorithm

import "github.com/khaledmdiab/horus_controller/core/model"

type MulticastAlgorithm interface {
	OnSessionActivated(session *model.Session) *model.MulticastTree

	OnSessionDeactivated(session *model.Session) *model.MulticastTree

	OnSessionCreated(session *model.Session) *model.MulticastTree

	OnReceiverAdded(session *model.Session, receiver *model.Node) *model.MulticastTree

	OnReceiverRemoved(session *model.Session, receiver *model.Node) *model.MulticastTree

	OnReceiverRemovedByAddress(session *model.Session, receiverAddress string) *model.MulticastTree
}
