package main

import (
	ctrl_leaf "github.com/khaledmdiab/horus_controller/ctrl_leaf"
)

func main() {
	cc := ctrl_leaf.NewController()
	cc.Run()
}
