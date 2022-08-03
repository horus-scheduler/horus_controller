package model

import (
	"fmt"
	"strings"
	"sync"

	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
)

/*
AsicRegistry
|
+-- Asic
	|
	+-- ID, Prog., DevID, PipeID, CtrlAddress, CtrlAPI
	+-- PortRegistry
		|
		+-- ConfigMap <key="user-defined", value=PortConfig>
		|	|
		|	+-- PortConfig
		|		|
		|		+-- ID (user-defined),
		|		+-- Speed, FEC, AN
		+-- PortMap <key="cage/lane", value=Port>
			|
			+-- Port
				|
				+-- Spec
				|	|
				|	+-- ID: read-only, set by Horus
				|	+-- Cage, Lane
				+-- devPort: read-only, set once by the centralized controller
				+-- Config: *PortConfig
				+-- Asic: *Asic
*/

type Asic struct {
	sync.RWMutex
	ID           string
	Program      string
	DeviceID     uint
	PipeID       uint
	CtrlAddress  string
	CtrlAPI      string
	PortRegistry *PortRegistry
}

func NewAsic(cfg *asicConfig, portConfigs []*portConfig, portGroups []*portGroup) *Asic {
	asic := &Asic{
		ID:          cfg.ID,
		Program:     cfg.Program,
		DeviceID:    cfg.DeviceID,
		PipeID:      cfg.PipeID,
		CtrlAddress: cfg.CtrlAddress,
		CtrlAPI:     cfg.CtrlAPI,
	}
	asic.PortRegistry = NewPortRegistry(asic, portConfigs, portGroups)
	return asic
}

func NewAsicFromInfo(asicInfo *horus_pb.AsicInfo, portConfigs []*horus_pb.PortConfigInfo) *Asic {
	asic := &Asic{
		ID:          asicInfo.ID,
		Program:     asicInfo.Program,
		DeviceID:    uint(asicInfo.DeviceID),
		PipeID:      uint(asicInfo.PipeID),
		CtrlAddress: asicInfo.CtrlAddress,
		CtrlAPI:     asicInfo.CtrlAPI,
	}
	asic.PortRegistry = NewPortRegistryFromInfo(asic, asicInfo, portConfigs)
	return asic
}

func (a *Asic) ToInfo() *horus_pb.AsicInfo {
	asicInfo := &horus_pb.AsicInfo{}
	asicInfo.ID = a.ID
	asicInfo.Program = a.Program
	asicInfo.DeviceID = uint32(a.DeviceID)
	asicInfo.PipeID = uint32(a.PipeID)
	asicInfo.CtrlAddress = a.CtrlAddress
	asicInfo.CtrlAPI = a.CtrlAPI

	for _, port := range a.PortRegistry.PortMap.Internal() {
		portInfo := port.ToInfo()
		asicInfo.PortsInfo = append(asicInfo.PortsInfo, portInfo)
	}

	return asicInfo
}

func (a *Asic) String() string {
	return fmt.Sprintf("ASIC ID=%s, Program=%s, DevID=%d, PipeID=%d, CtrlAddr=%s, API=%s",
		a.ID, a.Program, a.DeviceID, a.PipeID, a.CtrlAddress, a.CtrlAPI)
}

type AsicRegistry struct {
	AsicMap *AsicMap
}

func (ar *AsicRegistry) String() string {
	var lines []string
	for _, asic := range ar.AsicMap.Internal() {
		lines = append(lines, asic.String())
		lines = append(lines, asic.PortRegistry.String())
	}
	return strings.Join(lines, "\n")
}

func (ar *AsicRegistry) EncodeToPortConfigInfo() []*horus_pb.PortConfigInfo {
	var portConfsInfo []*horus_pb.PortConfigInfo
	for _, asic := range ar.AsicMap.Internal() {
		for _, portConf := range asic.PortRegistry.ConfigMap.Internal() {
			portConfInfo := &horus_pb.PortConfigInfo{}
			portConfInfo.ID = portConf.ID
			portConfInfo.Fec = portConf.Fec
			portConfInfo.Speed = portConf.Speed
			portConfInfo.An = portConf.An
			portConfsInfo = append(portConfsInfo, portConfInfo)
		}
		break
	}
	return portConfsInfo
}

func (ar *AsicRegistry) EncodeToAsicInfo() []*horus_pb.AsicInfo {
	var asicsInfo []*horus_pb.AsicInfo

	for _, asic := range ar.AsicMap.Internal() {
		asicInfo := asic.ToInfo()
		asicsInfo = append(asicsInfo, asicInfo)
	}
	return asicsInfo
}

func filterPortGroup(asicConfig *asicConfig, portGroups []*portGroup) []*portGroup {
	var groups []*portGroup
	for _, portGroup := range portGroups {
		if portGroup.Asic == asicConfig.ID {
			groups = append(groups, portGroup)
		}
	}
	return groups
}

func NewAsicRegistry(asicConfigs []*asicConfig, portConfigs []*portConfig, portGroups []*portGroup) *AsicRegistry {
	ar := &AsicRegistry{
		AsicMap: NewAsicMap(),
	}
	for _, cfg := range asicConfigs {
		asicPortGroups := filterPortGroup(cfg, portGroups)
		asic := NewAsic(cfg, portConfigs, asicPortGroups)
		_, found := ar.AsicMap.Load(cfg.ID)
		if !found {
			ar.AsicMap.Store(cfg.ID, asic)
		} else {
			logrus.Warnf("ASIC with ID=%s exists. Ignoring the new one.", cfg.ID)
		}
	}

	return ar
}

func NewAsicRegistryFromInfo(asicInfo []*horus_pb.AsicInfo,
	portConfigs []*horus_pb.PortConfigInfo) *AsicRegistry {
	ar := &AsicRegistry{
		AsicMap: NewAsicMap(),
	}
	for _, cfg := range asicInfo {
		asic := NewAsicFromInfo(cfg, portConfigs)
		_, found := ar.AsicMap.Load(cfg.ID)
		if !found {
			ar.AsicMap.Store(cfg.ID, asic)
		} else {
			logrus.Warnf("ASIC with ID=%s exists. Ignoring the new one.", cfg.ID)
		}
	}

	return ar
}

type AsicMap struct {
	sync.RWMutex
	internal map[string]*Asic
}

func NewAsicMap() *AsicMap {
	return &AsicMap{
		internal: make(map[string]*Asic),
	}
}

func (rm *AsicMap) Internal() map[string]*Asic {
	rm.Lock()
	defer rm.Unlock()
	return rm.internal
}

func (rm *AsicMap) Load(key string) (value *Asic, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *AsicMap) Delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *AsicMap) Store(key string, value *Asic) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}
