package main

import (
	ctrl_central "github.com/horus-scheduler/horus_controller/ctrl_central"
)

func main() {
	//algorithm := &ctl.MinBwAlgorithm{}
	//cc := ctrl_central.NewController()
	//cc.Run()

	//i := []byte("1a")
	//log.Println(i)

	//x := []byte{0x1a, 0x1b, 0x1c}
	//log.Println(x[1:3])
	//y := []byte{uint8(26), uint8(27), uint8(28)}
	//x_str := string(x)
	//log.Println([]byte(x_str))
	//log.Println(x)
	//
	//log.Println(string(y))
	//log.Println(y)

	cc := ctrl_central.NewCentralController()
	cc.Run()
}
