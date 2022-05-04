package ctrl_switch

import (
	"github.com/spf13/viper"
	"log"
	"os"
)

type config struct {
	TorId              uint32
	HealthyNodeTimeOut int64
	LocalRpcAddress    string
	RemoteRpcAddress   string
	LocalSockPath      string
	LocalSockType      string
}

func ReadConfigFile(configName string, configPaths ...string) *config {
	cfgName := "mdc-ctrl-switch"
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
	cfg.LocalRpcAddress = viper.GetString("rpc.local.servers.ctrlAddress")
	cfg.RemoteRpcAddress = viper.GetString("rpc.remote.ctrlAddress")

	sockPath := viper.GetString("tor.sockPath")
	if len(sockPath) > 2 && sockPath[1] == ':' {
		switch sockPath[0] {
		case 'u':
			cfg.LocalSockType = "unix"
		case 'r':
			cfg.LocalSockType = "raw"
		default:
			cfg.LocalSockType = ""
			log.Fatalf("Invalid tor.sockPath. It should be u:UNIX_SOCK_PATH or r:INTF_NAME\n")
			os.Exit(1)
		}
	} else {
		log.Fatalf("Invalid tor.sockPath. It should be u:UNIX_SOCK_PATH or r:INTF_NAME\n")
		os.Exit(1)
	}

	cfg.LocalSockPath = sockPath[2:]
	cfg.TorId = uint32(viper.GetInt32("tor.torId"))
	cfg.HealthyNodeTimeOut = viper.GetInt64("tor.healthyNodeTimeOut")
	return cfg
}
