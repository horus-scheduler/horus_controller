package main

import (
	"github.com/khaledmdiab/horus_controller/ctrl_leaf"
)

func main() {
	cc := ctrl_leaf.NewController()
	cc.Run()
}
