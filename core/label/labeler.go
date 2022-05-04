package label

import "github.com/khaledmdiab/horus_controller/core/model"

type Labeler interface {
	GetLabel(tree *model.MulticastTree) []byte
}
