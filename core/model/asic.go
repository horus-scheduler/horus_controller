package model

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

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
