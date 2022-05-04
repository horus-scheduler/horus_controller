package main

import (
	"log"
)

func main() {
	//algorithm := &ctl.MinBwAlgorithm{}
	//c := ctl.NewMteController("127.0.0.1:6675", 100, algorithm)
	//c.StartLoop()
	var x []int
	log.Println(x, len(x))
	x = append(x, 1, 2)
	log.Println(x, len(x))
}
