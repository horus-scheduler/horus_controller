package ctrl_central

import (
	"errors"
	"sync"

	"github.com/horus-scheduler/horus_controller/core/model"
	bfrtC "github.com/khaledmdiab/bfrt-go-client/pkg/client"
	"github.com/sirupsen/logrus"
)

type CentralizedCP interface {
	InitPorts()
	BringPortUp(*model.Port) error
	TakePortDown(*model.Port) error
	GetAsicStr() string
}

type FakeCentralizedCP struct {
	asicStr  string
	topology *model.Topology
}

func NewFakeCentralizedCP(asicStr string, topology *model.Topology) *FakeCentralizedCP {
	return &FakeCentralizedCP{asicStr: asicStr, topology: topology}
}

func (cp *FakeCentralizedCP) InitPorts() {
	logrus.Info("Calling FakeCentralizedCP.InitPorts")
}

func (cp *FakeCentralizedCP) GetAsicStr() string {
	return cp.asicStr
}

func (cp *FakeCentralizedCP) BringPortUp(port *model.Port) error {
	logrus.Info("Calling FakeCentralizedCP.BringPortUp")

	if port == nil {
		return errors.New("invalid port to be enabled")
	}

	if port.Asic == nil {
		return errors.New("invalid asic when enabling a port")
	}

	logrus.Infof("Port: %s is UP", port.Spec.ID)
	return nil
}

func (cp *FakeCentralizedCP) TakePortDown(port *model.Port) error {
	logrus.Info("Calling FakeCentralizedCP.TakePortDown")

	if port == nil {
		return errors.New("invalid port to be disabled")
	}

	logrus.Infof("Port: %s is DOWN", port.Spec.ID)
	return nil
}

type BfrtCentralizedCP struct {
	asicStr   string
	topology  *model.Topology
	clientMap *clientMap
}

func NewBfrtCentralizedCP(asicStr string, topology *model.Topology) *BfrtCentralizedCP {
	return &BfrtCentralizedCP{
		asicStr:   asicStr,
		topology:  topology,
		clientMap: newClientMap(),
	}
}

func (cp *BfrtCentralizedCP) GetAsicStr() string {
	return cp.asicStr
}

func (cp *BfrtCentralizedCP) InitPorts() {
	asicMap := cp.topology.AsicRegistry.AsicMap
	for _, asic := range asicMap.Internal() {
		logrus.Infof("Initialing ports of ASIC: %s", asic.ID)
		logrus.Warn("This may take a while")
		target := bfrtC.NewTarget(
			bfrtC.WithDeviceId(uint32(asic.DeviceID)),
			bfrtC.WithPipeId(uint32(asic.PipeID)),
		)
		client := bfrtC.NewClient(asic.CtrlAddress, asic.Program, uint32(5000), target)
		cp.clientMap.store(asic.ID, client)
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

func (cp *BfrtCentralizedCP) BringPortUp(port *model.Port) error {
	logrus.Info("Calling BfrtCentralizedCP.BringPortUp")
	if port == nil {
		return errors.New("invalid port to be enabled")
	}
	if port.Asic == nil {
		return errors.New("invalid asic when enabling a port")
	}

	if client, ok := cp.clientMap.load(port.Asic.ID); ok {
		return client.EnablePort(uint32(port.GetDevPort()),
			port.Config.Speed,
			port.Config.Fec,
			port.Config.An,
		)
	}
	return errors.New("bfrt client was not found during enabling the port")
}

func (cp *BfrtCentralizedCP) TakePortDown(port *model.Port) error {
	logrus.Info("Calling BfrtCentralizedCP.TakePortDown")
	if port == nil {
		return errors.New("invalid port to be disabled")
	}

	if port.Asic == nil {
		return errors.New("invalid asic when disabling a port")
	}

	if client, ok := cp.clientMap.load(port.Asic.ID); ok {
		return client.DisablePort(uint32(port.GetDevPort()))
	}
	return errors.New("bfrt client was not found during disabling the port")
}

type clientMap struct {
	sync.RWMutex
	internal map[string]*bfrtC.Client
}

func newClientMap() *clientMap {
	return &clientMap{
		internal: make(map[string]*bfrtC.Client),
	}
}

func (rm *clientMap) load(key string) (value *bfrtC.Client, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *clientMap) store(key string, value *bfrtC.Client) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}

//lint:ignore U1000 Ignore unused function to be used later
func (rm *clientMap) delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}
