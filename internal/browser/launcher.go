package browser

import (
	"github.com/go-rod/rod/lib/launcher"
	"github.com/spf13/viper"
)

func GetBrowserLauncher() *launcher.Launcher {
	options := launcher.New().
		Headless(viper.GetBool("crawl.headless")).
		Set("allow-running-insecure-content").
		Set("disable-infobars").
		Set("disable-extensions")

	if viper.GetString("navigation.proxy") != "" {
		options.Proxy(viper.GetString("navigation.proxy"))
	}
	if viper.GetBool("navigation.browser.disable_images") {
		options = options.Set("disable-images")
	}
	if viper.GetBool("navigation.browser.disable_gpu") {
		options = options.Set("disable-gpu")
	}
	return options
}
