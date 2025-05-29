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

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Warn().Msg("Config file not found, using defaults")
		} else {
			log.Panic().Err(err).Msg("Fatal error reading config file")
		}
	}
	SetDefaultConfig()
}

func SetDefaultConfig() {

	// Logging
	viper.SetDefault("logging.console.level", "info")
	viper.SetDefault("logging.console.format", "pretty") // if it's not pretty, just outputs json
	viper.SetDefault("logging.file.enabled", true)
	viper.SetDefault("logging.file.path", "sukyan.log")
	viper.SetDefault("logging.file.level", "info")

	// Database
	viper.SetDefault("db.max_idle_conns", 5)
	viper.SetDefault("db.max_open_conns", 50)
	viper.SetDefault("db.conn_max_lifetime", "1h")

	// Storage
	viper.SetDefault("history.responses.ignored.max_size", 5*1024*1024)
	viper.SetDefault("history.responses.ignored.extensions", []string{".jpg", ".jpeg", ".webp", ".png", ".gif", ".ico", ".mp4", ".mov", ".avi"})
	viper.SetDefault("history.responses.ignored.content_types", []string{"video", "audio", "image"})

	// Navigation
	viper.SetDefault("navigation.user_agent", "")
	viper.SetDefault("navigation.timeout", 10)
	viper.SetDefault("navigation.wait_stable", true)
	viper.SetDefault("navigation.wait_stable_duration", 2)
	viper.SetDefault("navigation.wait_stable_timeout", 10)

	viper.SetDefault("navigation.max_redirects", 10)
	viper.SetDefault("navigation.proxy", "")
	viper.SetDefault("navigation.auth.basic.username", "admin")
	viper.SetDefault("navigation.auth.basic.password", "password")
	viper.SetDefault("navigation.browser.disable_images", false)
	viper.SetDefault("navigation.browser.disable_gpu", true)

	// Crawl
	viper.SetDefault("crawl.headless", true)
	viper.SetDefault("crawl.page_setup_timeout", 15)
	viper.SetDefault("crawl.interaction.timeout", 10)
	viper.SetDefault("crawl.interaction.submit_forms", true)
	viper.SetDefault("crawl.interaction.click_buttons", true)
	viper.SetDefault("crawl.common.files", []string{"/robots.txt", "/sitemap.xml"})
	viper.SetDefault("crawl.ignored_extensions", []string{".jpg", ".woff2", ".png", ".gif", ".webp", ".ico", ".css", ".svg", ".tif", ".tiff", ".bmp", ".raw", ".indd", ".ai", ".eps", ".pdf", ".exe", ".dll", ".psd", ".fla", ".avi", ".flv", ".mov", ".mp4", ".mpg", ".mpeg", ".swf", ".mkv", ".wav", ".mp3", ".flac", ".m4a", ".wma", ".aac", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".rtf", ".zip", ".rar", ".7z", ".tar.gz", ".iso", ".dmg"})
	viper.SetDefault("crawl.max_pages_with_same_params", 20)

	// Scan
	viper.SetDefault("scan.magic_words", []string{"null", "None", "Undefined", "Blank"})
	viper.SetDefault("scan.concurrency.passive", 30)
	viper.SetDefault("scan.concurrency.active", 15)
	viper.SetDefault("scan.browser.pool_size", 6)

	viper.SetDefault("scan.oob.enabled", true)
	viper.SetDefault("scan.oob.poll_interval", 10)
	viper.SetDefault("scan.oob.wait_after_scan", 30)
	viper.SetDefault("scan.oob.asn_info", false)
	viper.SetDefault("scan.oob.server_urls", "oast.pro,oast.live,oast.site,oast.online,oast.fun,oast.me")

	viper.SetDefault("scan.avoid_repeated_issues", true)

	// Generators
	viper.SetDefault("generators.directory", "/etc/sukyan/generators")

	// Passive
	viper.SetDefault("passive.checks.headers.enabled", true)
	viper.SetDefault("passive.checks.js.enabled", true)
	viper.SetDefault("passive.checks.missconfigurations.enabled", true)
	viper.SetDefault("passive.checks.exceptions.enabled", true)

	// Forms
	viper.SetDefault("forms.auto_fill", true)
	viper.SetDefault("forms.auto_fill.types.text", "aa")
	viper.SetDefault("forms.auto_fill.types.password", "password")
	viper.SetDefault("forms.auto_fill.types.email", "")
	viper.SetDefault("forms.auto_fill.types.number", "123")
	viper.SetDefault("forms.auto_fill.types.search", "search")
	viper.SetDefault("forms.auto_fill.types.tel", "1234567890")
	viper.SetDefault("forms.auto_fill.types.url", "http://www.example.com")
	viper.SetDefault("forms.auto_fill.types.week", "2023-W24")
	viper.SetDefault("forms.auto_fill.types.color", "#ffffff")
	viper.SetDefault("forms.auto_fill.types.checkbox", "true")
	viper.SetDefault("forms.auto_fill.types.radio", "option1")
	viper.SetDefault("forms.auto_fill.types.range", "50")
	viper.SetDefault("forms.auto_fill.types.hidden", "defaultHidden")

	viper.SetDefault("forms.auto_fill.names.username", "admin")
	viper.SetDefault("forms.auto_fill.names.password", "password")
	viper.SetDefault("forms.auto_fill.names.email", "example@example.com")

	// Integrations
	viper.SetDefault("integrations.nuclei.enabled", true)
	viper.SetDefault("integrations.nuclei.host", "localhost")
	viper.SetDefault("integrations.nuclei.port", 8555)
	viper.SetDefault("integrations.nuclei.scan_timeout", 30)
	viper.SetDefault("integrations.nuclei.automatic_scan", true)
	viper.SetDefault("integrations.nuclei.include_ids", []string{})
	viper.SetDefault("integrations.nuclei.exclude_ids", []string{"http-missing-security-headers"})
	viper.SetDefault("integrations.nuclei.tags", []string{})
	viper.SetDefault("integrations.nuclei.exclude_tags", []string{})
	viper.SetDefault("integrations.nuclei.workflows", []string{})
	viper.SetDefault("integrations.nuclei.exclude_workflows", []string{})
	viper.SetDefault("integrations.nuclei.templates", []string{})
	viper.SetDefault("integrations.nuclei.excluded_templates", []string{})
	viper.SetDefault("integrations.nuclei.authors", []string{})
	viper.SetDefault("integrations.nuclei.exclude_matchers", []string{})
	viper.SetDefault("integrations.nuclei.severities", []string{})
	viper.SetDefault("integrations.nuclei.exclude_severities", []string{})
	viper.SetDefault("integrations.nuclei.protocols", []string{})
	viper.SetDefault("integrations.nuclei.exclude_protocols", []string{})

	viper.SetDefault("wordlists.directory", "/etc/sukyan/wordlists")
	viper.SetDefault("wordlists.extensions", []string{".txt", ".lst", ".wordlist", ".list", "wordlists"})

	viper.SetDefault("server.cert.file", "server.crt")
	viper.SetDefault("server.key.file", "server.key")
	viper.SetDefault("server.caCert.file", "ca.crt")
	viper.SetDefault("server.caKey.file", "ca.key")
	viper.SetDefault("server.cert.organization", "Sukyan")
	viper.SetDefault("server.cert.country", "XX")
	viper.SetDefault("server.cert.locality", "XXX")
	viper.SetDefault("server.cert.street_address", "")
	viper.SetDefault("server.cert.postal_code", "")

	// API
	viper.SetDefault("api.listen.host", "")
	viper.SetDefault("api.listen.port", 8013)
	viper.SetDefault("api.docs.enabled", false)
	viper.SetDefault("api.docs.path", "/docs")
	viper.SetDefault("api.metrics.enabled", false)
	viper.SetDefault("api.metrics.path", "/metrics")
	viper.SetDefault("api.metrics.title", "Sukyan Metrics")
	viper.SetDefault("api.pprof.enabled", false)
	viper.SetDefault("api.pprof.prefix", "")

	viper.SetDefault("api.cors.origins", []string{"http://localhost:3001", "http://127.0.0.1:3001"})
	viper.SetDefault("api.auth.jwt_secret_key", "ch4ng3Th1sToAS3cr3tK3y")
	viper.SetDefault("api.auth.jwt_secret_expire_minutes", 15)
	viper.SetDefault("api.auth.jwt_refresh_key", "ch4ng3Th1sK3y")
	viper.SetDefault("api.auth.jwt_refresh_expire_hours", 7*24)
}
