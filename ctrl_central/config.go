package ctrl_central

import (
	"github.com/spf13/viper"
	"log"
)

type config struct {
	AppServer    string
	TorServer    string
	TorCount     int
	TorAddresses []string
}

func ReadConfigFile(configName string, configPaths ...string) *config {
	cfgName := "mdc-ctrl-central"
	if configName != "" {
		cfgName = configName
	}
	viper.SetConfigName(cfgName)
	viper.AddConfigPath("/etc/mdc/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.mdc") // call multiple times to add many search paths
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
