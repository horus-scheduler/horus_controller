package main

import "github.com/khaledmdiab/horus_controller/ctrl_spine"

func main() {
	cc := ctrl_spine.NewController()
	cc.Run()
}
