package ctrl_sw

import (
	"log"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type bfrtContext struct {
	DeviceID    uint32
	PipeID      uint32
	P4Name      string
	BfrtAddress string
}

type rootConfig struct {
	bfrtContext
	AsicIntf string
	SpineIDs []uint16
	LeafIDs  []uint16
	Timeout  int64
}

func ReadConfigFile(configName string, configPaths ...string) *rootConfig {
	if configName != "" {
		logrus.Debugf("Using the provided manager configuration file %s", configName)
		viper.SetConfigFile(configName)
	} else {
		logrus.Debugf("Using the default manager configuration file")
		cfgName := "horus-ctrl-sw"

		viper.SetConfigName(cfgName)
		viper.AddConfigPath("/etc/horus/")  // path to look for the config file in
		viper.AddConfigPath("$HOME/.horus") // call multiple times to add many search paths
		viper.AddConfigPath(".")
		viper.AddConfigPath("./conf")
		for _, confPath := range configPaths {
			viper.AddConfigPath(confPath)
		}
	}

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		log.Fatalf("Fatal error config file: %s \n", err)
	}
	cfg := &rootConfig{}
	cfg.AsicIntf = viper.GetString("asic.intf")
	cfg.P4Name = viper.GetString("asic.program")
	cfg.DeviceID = viper.GetUint32("asic.device_id")
	cfg.PipeID = viper.GetUint32("asic.pipe_id")
	cfg.BfrtAddress = viper.GetString("bfrt.address")
	cfg.Timeout = viper.GetInt64("controllers.timeout")
	err = viper.UnmarshalKey("controllers.spines", &cfg.SpineIDs)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}
	err = viper.UnmarshalKey("controllers.leaves", &cfg.LeafIDs)
	if err != nil {
		logrus.Errorf("unable to decode into struct, %v", err)
		err = nil
	}

	return cfg
}
