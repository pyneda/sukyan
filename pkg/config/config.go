package config

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func LoadConfig() {
	viper.SetConfigName("config")       // name of config file (without extension)
	viper.SetConfigType("yaml")         // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath("/etc/sukyan/") // path to look for the config file in
	viper.AddConfigPath(".")            // optionally look for config in the working directory
	err := viper.ReadInConfig()         // Find and read the config file
	if err != nil {                     // Handle errors reading the config file
		//panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
			log.Warn().Msg("Config file not found")
		} else {
			// Config file was found but another error was produced
			log.Panic().Err(err).Msg("Fatal error reading config file")
		}
	}
	SetDefaultConfig()
}

func SetDefaultConfig() {
	viper.SetDefault("workspace.id", 1)
	viper.SetDefault("scan.magic_words", []string{"null", "None", "Undefined", "Blank"})
	viper.SetDefault("scan.crawl.enabled", false)
	viper.SetDefault("scan.concurrency.max_audits", 4)
	viper.SetDefault("scan.concurrency.per_browser_audit", 4)
	viper.SetDefault("scan.concurrency.per_http_audit", 16)
}
