package main

import (
	"flag"

	ctrl_sw "github.com/khaledmdiab/horus_controller/ctrl_sw"
	"github.com/sirupsen/logrus"
)

func main() {

	var topoFp string
	var vcsFp string
	var cfgFp string

	flag.StringVar(&topoFp, "topo", "", "Topology file path")
	flag.StringVar(&vcsFp, "vcs", "", "Virtual clusters file path")
	flag.StringVar(&cfgFp, "cfg", "", "Manager configuration file path")
	flag.Parse()

	if topoFp == "" {
		logrus.Fatal("Topology file path is required")
	}

	if vcsFp == "" {
		logrus.Fatal("Virtual clusters file path is required")
	}

	if cfgFp == "" {
		logrus.Warn("Manager configuration file path is empty")
	}

	cc := ctrl_sw.NewSwitchManager(topoFp, vcsFp, cfgFp)
	cc.Run()
}
