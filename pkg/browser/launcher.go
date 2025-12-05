package browser

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/spf13/viper"
)

func GetBrowserLauncher() *launcher.Launcher {
	options := launcher.New().
		Headless(viper.GetBool("crawl.headless")).
		Set("allow-running-insecure-content").
		Set("disable-infobars").
		Set("disable-extensions").
		Set("no-sandbox")

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

func NewBrowser() *rod.Browser {
	launcher := GetBrowserLauncher()
	controlURL := launcher.MustLaunch()
	return rod.New().ControlURL(controlURL).MustConnect()
}

// NewBrowserWithTimeout attempts to create a new browser instance with a specified timeout.
func NewBrowserWithTimeout(timeoutDuration time.Duration) (*rod.Browser, error) {
	type result struct {
		browser *rod.Browser
		err     error
	}

	resultChan := make(chan result, 1)

	go func() {
		launcher := GetBrowserLauncher()
		controlURL, err := launcher.Launch()
		if err != nil {
			resultChan <- result{nil, err}
			return
		}
		b := rod.New().ControlURL(controlURL).MustConnect()
		resultChan <- result{browser: b}
	}()

	select {
	case res := <-resultChan:
		return res.browser, res.err
	case <-time.After(timeoutDuration):
		return nil, fmt.Errorf("timeout reached while trying to launch a browser")
	}
}
