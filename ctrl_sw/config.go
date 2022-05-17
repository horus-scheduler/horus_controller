package ctrl_sw

import (
	"fmt"
	"log"

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
	AsicIntf    string
	Controllers []ctrlConfig
}

type ctrlConfig struct {
	ID      string
	Type    string
	Address string
}

func ReadConfigFile(configName string, configPaths ...string) *rootConfig {
	cfgName := "horus-ctrl-sw"
	if configName != "" {
		cfgName = configName
	}
	viper.SetConfigName(cfgName)
	viper.AddConfigPath("/etc/horus/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.horus") // call multiple times to add many search paths
	viper.AddConfigPath(".")
	viper.AddConfigPath("./conf")
	for _, confPath := range configPaths {
		viper.AddConfigPath(confPath)
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

	err = viper.UnmarshalKey("controllers", &cfg.Controllers)
	if err != nil {
		fmt.Printf("unable to decode into struct, %v", err)
		err = nil
	}

	return cfg
}
