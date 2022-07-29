package model

import (
	"log"

	horus_pb "github.com/horus-scheduler/horus_controller/protobuf"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type BinRootConfig struct {
	SrvServer string
	// VCServer   string
	LogLevel string
}

type asicRootConfig struct {
	Asics []*asicConfig `mapstructure:"asic"`
}

type portRootConfig struct {
	PortConfig []*portConfig `mapstructure:"config"`
	PortGroup  []*portGroup  `mapstructure:"group"`
}

type topoRootConfig struct {
	Clients []*clientConfig
	Spines  []*spineConfig
	Leaves  []*leafConfig
	Servers []*serverConfig
}

type portConfig struct {
	ID    string
	Speed string
	Fec   string
	An    string
}

type asicConfig struct {
	ID          string
	Program     string
	DeviceID    uint   `mapstructure:"deviceId"`
	PipeID      uint   `mapstructure:"pipeId"`
	CtrlAddress string `mapstructure:"ctrlAddress"`
	CtrlAPI     string `mapstructure:"ctrlAPI"`
}

type portGroup struct {
	Specs  []string
	Config string
	Asic   string
}

type clientConfig struct {
	ID   uint16
	Port string
}

type spineConfig struct {
	ID      uint16
	Address string
	LeafIDs []uint16 `mapstructure:"leaves"`
	Asic    string
}

// Parham: Please check, added PortID
type leafConfig struct {
	ID          uint16
	Index       uint16
	Address     string
	MgmtAddress string   `mapstructure:"mgmtAddress"`
	ServerIDs   []uint16 `mapstructure:"servers"`
	Asic        string
	PortID      uint16 `mapstructure:"port_id"`
	UsPort      string `mapstructure:"us_port"`
	DsPort      string `mapstructure:"ds_port"`
}

type serverConfig struct {
	ID           uint16
	PortID       uint16 `mapstructure:"port_id"`
	Port         string `mapstructure:"port"`
	Address      string
	WorkersCount uint16 `mapstructure:"workers_count"`
}

type vcRootConfig struct {
	VCs []*horus_pb.VCInfo
}

func setCommonPaths(configName string, configPaths ...string) {
	viper.SetConfigName(configName)
	viper.AddConfigPath("/etc/horus/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.horus") // call multiple times to add many search paths
	viper.AddConfigPath(".")
	viper.AddConfigPath("./conf")
	for _, confPath := range configPaths {
		viper.AddConfigPath(confPath)
	}
}

func ReadConfigFile(configName string, configPaths ...string) *BinRootConfig {
	cfgName := "horus-ctrl-central"
	setCommonPaths(cfgName, configPaths...)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		log.Fatalf("Fatal error config file: %s \n", err)
	}
	cfg := &BinRootConfig{}
	cfg.SrvServer = viper.GetString("server.srvAddress")
	cfg.LogLevel = viper.GetString("log.level")

	return cfg
}

func ReadTopologyFile(configName string, configPaths ...string) *topoRootConfig {
	cfgName := "horus-topology"
	setCommonPaths(cfgName, configPaths...)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		log.Fatalf("Fatal error config file: %s \n", err)
	}
	topCfg := &topoRootConfig{}
	asicsCfg := &asicRootConfig{}
	portCfg := &portRootConfig{}

	err = viper.UnmarshalKey("topology", &topCfg)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}
	err = viper.UnmarshalKey("ports", &portCfg)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}
	err = viper.UnmarshalKey("asics", &asicsCfg)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}
	ar := NewAsicRegistry(asicsCfg.Asics, portCfg.PortConfig, portCfg.PortGroup)
	logrus.Info(ar)
	logrus.Info(topCfg.Leaves[0])
	logrus.Fatal("X")

	return topCfg
}

func ReadVCsFile(configName string, configPaths ...string) *vcRootConfig {
	cfgName := "horus-vcs"
	setCommonPaths(cfgName, configPaths...)

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		logrus.Fatalf("Fatal error config file: %s \n", err)
	}
	cfg := &vcRootConfig{}
	err = viper.UnmarshalKey("vc", &cfg.VCs)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}

	return cfg
}
