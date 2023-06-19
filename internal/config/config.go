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

	// Navigation
	viper.SetDefault("navigation.user_agent", "")
	viper.SetDefault("navigation.timeout", 15)
	viper.SetDefault("navigation.max_retries", 3)
	viper.SetDefault("navigation.retry_delay", 5)
	viper.SetDefault("navigation.max_redirects", 10)
	viper.SetDefault("navigation.headers", map[string]string{})
	viper.SetDefault("navigation.cookies", map[string]string{})
	viper.SetDefault("navigation.proxy_auth", "")
	viper.SetDefault("navigation.proxy_type", "http")
	viper.SetDefault("navigation.proxy_user", "")
	viper.SetDefault("navigation.proxy_pass", "")

	// Crawl
	viper.SetDefault("crawl.max_depth", 10)
	viper.SetDefault("crawl.pool_size", 4)
	viper.SetDefault("crawl.headless", true)
	viper.SetDefault("crawl.interaction.submit_forms", true)
	viper.SetDefault("crawl.interaction.click_buttons", true)
	viper.SetDefault("crawl.interaction.timeout", 5)

	// Scan
	viper.SetDefault("scan.magic_words", []string{"null", "None", "Undefined", "Blank"})
	viper.SetDefault("scan.crawl.enabled", false)
	viper.SetDefault("scan.concurrency.max_audits", 4)
	viper.SetDefault("scan.concurrency.per_browser_audit", 4)
	viper.SetDefault("scan.concurrency.per_http_audit", 16)
	viper.SetDefault("scan.passive.wappalyzer", false)
	viper.SetDefault("scan.passive.retirejs", false)

	// API
	viper.SetDefault("api.listen.host", "")
	viper.SetDefault("api.listen.port", 8013)
}
