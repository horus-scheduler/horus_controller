package ctrl_central

import (
	"log"

	"github.com/spf13/viper"
)

type config struct {
	AppServer    string
	TorServer    string
	TorCount     int
	TorAddresses []string
}

func ReadConfigFile(configName string, configPaths ...string) *config {
	cfgName := "horus-ctrl-central"
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
	cfg := &config{}
	cfg.AppServer = viper.GetString("rpc.local.servers.appServerAddress")
	cfg.TorServer = viper.GetString("rpc.local.servers.torServerAddress")
	cfg.TorAddresses = viper.GetStringSlice("tors.addresses")
	return cfg
}
