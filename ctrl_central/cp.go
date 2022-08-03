package ctrl_central

import (
	"github.com/horus-scheduler/horus_controller/core/model"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
)

type CentralizedCP interface {
	InitPorts()
}

type FakeCentralizedCP struct {
	topology *model.Topology
}

func NewFakeCentralizedCP(topology *model.Topology) *FakeCentralizedCP {
	return &FakeCentralizedCP{topology: topology}
}

func (cp *FakeCentralizedCP) InitPorts() {
	logrus.Info("Calling FakeCentralizedCP.InitPorts")
}

type BfrtCentralizedCP struct {
	topology *model.Topology
}

func NewBfrtCentralizedCP(topology *model.Topology) *BfrtCentralizedCP {
	return &BfrtCentralizedCP{
		topology: topology,
	}
}

func (cp *BfrtCentralizedCP) InitPorts() {
	asicMap := cp.topology.AsicRegistry.AsicMap
	for _, asic := range asicMap.Internal() {
		logrus.Infof("Initialing ports of ASIC: %s", asic.ID)
		logrus.Warn("This may take time")
		target := bfrtC.NewTarget(bfrtC.WithDeviceId(uint32(asic.DeviceID)),
			bfrtC.WithPipeId(uint32(asic.PipeID)))
		client := bfrtC.NewClient(asic.CtrlAddress, asic.Program, uint32(1000), target)
		portMap := asic.PortRegistry.PortMap
		for _, port := range portMap.Internal() {
			devPort, err := client.GetDevPort(port.Spec.Cage, port.Spec.Lane)
			if err != nil {
				logrus.Fatal(err)
			}
			port.SetDevPort(uint64(devPort))
			err = client.EnablePort(devPort, port.Config.Speed, port.Config.Fec, port.Config.An)
			if err != nil {
				logrus.Fatal(err)
			}
		}
		logrus.Infof("Finished initializing the ports of ASIC: %s", asic.ID)
	}
}
