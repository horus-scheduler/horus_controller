package model

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

var BfSpeedMap = map[string]string{
	"1G":   "BF_SPEED_1G",
	"10G":  "BF_SPEED_10G",
	"25G":  "BF_SPEED_25G",
	"40G":  "BF_SPEED_40G",
	"50G":  "BF_SPEED_50G",
	"100G": "BF_SPEED_100G",
	"200G": "BF_SPEED_200G",
	"400G": "BF_SPEED_400G",
}

var BfFecMap = map[string]string{
	"NONE": "BF_FEC_TYP_NONE",
	"FC":   "BF_FEC_TYP_FIRECODE",
	"RS":   "BF_FEC_TYP_REED_SOLOMON",
}

var BfAnMap = map[string]string{
	"AUTO":     "PM_AN_DEFAULT",
	"ENABLED":  "PM_AN_FORCE_ENABLE",
	"DISABLED": "PM_AN_FORCE_DISABLE",
}

type PortConfig struct {
	ID    string
	Speed string
	Fec   string
	An    string
}

func (pc *PortConfig) String() string {
	return fmt.Sprintf("PortConfig ID=%s, Speed=%s, Fec=%s, An=%s", pc.ID, pc.Speed, pc.Fec, pc.An)
}

type Port struct {
	Spec    *PortSpec
	DevPort uint64
	Config  *PortConfig
}

func (p *Port) String() string {
	return fmt.Sprintf("%s, DevPort=%d (%s)", p.Spec, p.DevPort, p.Config)
}

type PortSpec struct {
	ID   string
	Cage uint64
	Lane uint64
}

func (p *PortSpec) String() string {
	return fmt.Sprintf("PortSpec ID=%s", p.ID)
}

func NewPortConfig(portConfig *portConfig) (*PortConfig, error) {
	bfSpeedKey := strings.ToUpper(portConfig.Speed)
	bfFecKey := strings.ToUpper(portConfig.Fec)
	bfAnKey := strings.ToUpper(portConfig.An)

	bfSpeed, ok1 := BfSpeedMap[bfSpeedKey]
	bfFec, ok2 := BfFecMap[bfFecKey]
	bfAn, ok3 := BfAnMap[bfAnKey]

	if portConfig.ID != "" && ok1 && ok2 && ok3 {
		return &PortConfig{
			ID:    portConfig.ID,
			Speed: bfSpeed,
			Fec:   bfFec,
			An:    bfAn,
		}, nil
	}

	return nil, fmt.Errorf("port config ID=%s, Speed=%s, Fec=%s, AN=%s is invalid",
		portConfig.ID, bfSpeedKey, bfFecKey, bfAnKey)
}

func NewPortSpec(cage uint64, lane uint64) *PortSpec {
	return &PortSpec{
		ID:   fmt.Sprintf("%d/%d", cage, lane),
		Cage: cage,
		Lane: lane,
	}
}

func NewPort(spec *PortSpec, devPort uint64, config *PortConfig) *Port {
	return &Port{
		Spec:    spec,
		DevPort: devPort,
		Config:  config,
	}
}

type PortRegistry struct {
	ConfigMap *PortConfigMap
	PortMap   *PortMap
}

func (pr *PortRegistry) String() string {
	var lines []string
	for _, port := range pr.PortMap.Internal() {
		lines = append(lines, port.String())
	}
	return strings.Join(lines, "\n")
}

func NewPortRegistry(portConfigs []*portConfig, portGroups []*portGroup) *PortRegistry {
	pr := &PortRegistry{
		ConfigMap: NewPortConfigMap(),
		PortMap:   NewPortMap(),
	}
	for _, cfg := range portConfigs {
		portCfg, err := NewPortConfig(cfg)
		if err == nil {
			_, found := pr.ConfigMap.Load(portCfg.ID)
			if !found {
				pr.ConfigMap.Store(portCfg.ID, portCfg)
			} else {
				logrus.Warnf("PortConfig with ID=%s exists. Ignoring the new one.", portCfg.ID)
			}
		} else {
			logrus.Warn(err)
		}
	}

	for _, port := range portGroups {
		var allSpecs []*PortSpec
		for _, specStr := range port.Specs {
			portSpecs, err := ExpandPortSpec(specStr)
			allSpecs = append(allSpecs, portSpecs...)
			if err != nil {
				logrus.Warn(err)
			}
		}
		cfgID := port.Config
		if portCfg, found := pr.ConfigMap.Load(cfgID); found {
			for _, spec := range allSpecs {
				_, found := pr.PortMap.Load(spec.ID)
				if !found {
					port := NewPort(spec, 0, portCfg)
					pr.PortMap.Store(port.Spec.ID, port)
				} else {
					logrus.Warnf("PortSpec with ID=%s exists. Ignoring the new one.", spec.ID)
				}
			}
		} else {
			logrus.Warnf("PortConfig with ID=%s doesn't exists.", cfgID)
		}
	}
	return pr
}

func ExpandPortSpec(spec string) ([]*PortSpec, error) {
	trimmedSpec := strings.TrimSpace(spec)
	if !strings.Contains(trimmedSpec, "/") {
		return nil, fmt.Errorf("spec %s is invalid: expected cage/lane format", spec)
	}

	splitted := strings.Split(trimmedSpec, "/")
	if len(splitted) != 2 {
		return nil, fmt.Errorf("spec %s is invalid: expected cage/lane format", spec)
	}

	cage, err := strconv.Atoi(splitted[0])
	if err != nil {
		return nil, fmt.Errorf("spec %s is invalid: %s", spec, err.Error())
	}

	if splitted[1] == "-" {
		var specs []*PortSpec
		specs = append(specs, NewPortSpec(uint64(cage), 0))
		specs = append(specs, NewPortSpec(uint64(cage), 1))
		specs = append(specs, NewPortSpec(uint64(cage), 2))
		specs = append(specs, NewPortSpec(uint64(cage), 3))
		return specs, nil
	}
	lane, err := strconv.Atoi(splitted[1])
	if err != nil {
		return nil, fmt.Errorf("spec %s is invalid: %s", spec, err.Error())
	}
	if lane < 0 || lane > 3 {
		return nil, fmt.Errorf("spec %s is invalid: 0 <= lane <= 3", spec)
	}
	return []*PortSpec{NewPortSpec(uint64(cage), uint64(lane))}, nil
}

type PortConfigMap struct {
	sync.RWMutex
	internal map[string]*PortConfig
}

func NewPortConfigMap() *PortConfigMap {
	return &PortConfigMap{
		internal: make(map[string]*PortConfig),
	}
}

func (rm *PortConfigMap) Internal() map[string]*PortConfig {
	rm.Lock()
	defer rm.Unlock()
	return rm.internal
}

func (rm *PortConfigMap) Load(key string) (value *PortConfig, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *PortConfigMap) Delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *PortConfigMap) Store(key string, value *PortConfig) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}

type PortMap struct {
	sync.RWMutex
	internal map[string]*Port
}

func NewPortMap() *PortMap {
	return &PortMap{
		internal: make(map[string]*Port),
	}
}

func (rm *PortMap) Internal() map[string]*Port {
	rm.Lock()
	defer rm.Unlock()
	return rm.internal
}

func (rm *PortMap) Load(key string) (value *Port, ok bool) {
	rm.RLock()
	result, ok := rm.internal[key]
	rm.RUnlock()
	return result, ok
}

func (rm *PortMap) Delete(key string) {
	rm.Lock()
	delete(rm.internal, key)
	rm.Unlock()
}

func (rm *PortMap) Store(key string, value *Port) {
	rm.Lock()
	rm.internal[key] = value
	rm.Unlock()
}
