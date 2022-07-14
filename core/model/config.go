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

type topoRootConfig struct {
	Spines  []*spineConfig
	Leaves  []*leafConfig
	Servers []*serverConfig
}

type spineConfig struct {
	ID      uint16
	Address string
	LeafIDs []uint16 `mapstructure:"leaves"`
}

// Parham: Please check, added PortID
type leafConfig struct {
	ID          uint16
	Index       uint16
	Address     string
	PortID      uint16   `mapstructure:"port_id"`
	MgmtAddress string   `mapstructure:"mgmtAddress"`
	ServerIDs   []uint16 `mapstructure:"servers"`
}

type serverConfig struct {
	ID           uint16
	PortID       uint16 `mapstructure:"port_id"`
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
	cfg := &topoRootConfig{}

	err = viper.UnmarshalKey("topology", &cfg)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}

	return cfg
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
