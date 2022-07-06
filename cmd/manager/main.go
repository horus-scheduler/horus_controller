package main

import (
	"flag"

	ctrl_mgr "github.com/horus-scheduler/horus_controller/ctrl_mgr"
	"github.com/sirupsen/logrus"
)

func main() {
	var topoFp string
	var cfgFp string

	flag.StringVar(&topoFp, "topo", "", "Topology file path")
	flag.StringVar(&cfgFp, "cfg", "", "Manager configuration file path")
	flag.Parse()

	if topoFp == "" {
		logrus.Fatal("Topology file path is required")
	}

	if cfgFp == "" {
		logrus.Warn("Manager configuration file path is empty")
	}

	cc := ctrl_mgr.NewSwitchManager(topoFp, cfgFp)
	cc.Run()
}
