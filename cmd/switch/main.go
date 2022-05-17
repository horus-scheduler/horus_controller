package main

import (
	ctrl_sw "github.com/khaledmdiab/horus_controller/ctrl_sw"
)

func main() {
	cc := ctrl_sw.NewController()
	cc.Run()
}
