package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

type ScreenshotResult struct {
	Selector   string `json:"selector"`
	Data       string `json:"data,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
}

type ActionsExecutionResults struct {
	Screenshots []ScreenshotResult `json:"screenshots"`
	// To add:
	// Logs
	// Errors
	// JsEvaluationResults
}

func ExecuteActions(ctx context.Context, page *rod.Page, actions []Action) (ActionsExecutionResults, error) {
	var results ActionsExecutionResults
	for _, action := range actions {
		select {
		case <-ctx.Done():
			return results, ctx.Err() // Return context cancellation error
		default:
		}

		switch action.Type {
		case ActionNavigate:
			err := page.Navigate(action.URL)
			if err != nil {
				return results, fmt.Errorf("failed to navigate to %s: %w", action.URL, err)
			}
			err = page.WaitLoad()
			if err != nil {
				return results, fmt.Errorf("failed to wait for page load: %w", err)
			}
			log.Info().Str("url", action.URL).Msg("Browser action navigated to URL")

		case ActionWait:
			el, err := page.Element(action.Selector)
			if err != nil {
				return results, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			switch action.For {
			case WaitVisible:
				err = el.WaitVisible()
			case WaitHidden:
				err = el.WaitInvisible()
			case WaitEnabled:
				err = el.WaitEnabled()
			case WaitLoad:
				err = page.WaitLoad()
			}
			if err != nil {
				return results, fmt.Errorf("failed to wait for element %s: %w", action.Selector, err)
			}
			log.Info().Str("selector", action.Selector).Msg("Browser action waited for element")

		case ActionClick:
			el, err := page.Element(action.Selector)
			if err != nil {
				return results, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			err = el.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return results, fmt.Errorf("failed to click element %s: %w", action.Selector, err)
			}
			log.Info().Str("selector", action.Selector).Msg("Browser action clicked element")

		case ActionFill:
			el, err := page.Element(action.Selector)
			if err != nil {
				return results, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			err = el.Input(action.Value)
			if err != nil {
				return results, fmt.Errorf("failed to fill element %s with value %s: %w", action.Selector, action.Value, err)
			}
			log.Info().Str("selector", action.Selector).Str("value", action.Value).Msg("Browser action filled element")

		case ActionAssert:
			el, err := page.Element(action.Selector)
			if err != nil {
				return results, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			text, err := el.Text()
			if err != nil {
				return results, fmt.Errorf("failed to get text of element %s: %w", action.Selector, err)
			}
			switch action.Condition {
			case AssertContains:
				if !strings.Contains(text, action.Value) {
					return results, fmt.Errorf("assertion failed: element text does not contain '%s'", action.Value)
				}
				log.Info().Str("selector", action.Selector).Str("value", action.Value).Msg("Browser action contains assertion passed")
			case AssertEquals:
				if text != action.Value {
					return results, fmt.Errorf("assertion failed: element text is not equal to '%s'", action.Value)
				}
				log.Info().Str("selector", action.Selector).Str("value", action.Value).Msg("Browser action equals assertion passed")
			case AssertVisible:
				isVisible, err := el.Visible()
				if err != nil {
					return results, fmt.Errorf("failed to check visibility of element %s: %w", action.Selector, err)
				}
				if !isVisible {
					return results, fmt.Errorf("assertion failed: element %s is not visible", action.Selector)
				}
				log.Info().Str("selector", action.Selector).Msg("Browser action visible assertion passed")

			case AssertHidden:
				isVisible, err := el.Visible()
				if err != nil {
					return results, fmt.Errorf("failed to check visibility of element %s: %w", action.Selector, err)
				}
				if isVisible {
					return results, fmt.Errorf("assertion failed: element %s is visible, expected hidden", action.Selector)
				}
				log.Info().Str("selector", action.Selector).Msg("Browser action hidden assertion passed")
			}

		case ActionScroll:
			el, err := page.Element(action.Selector)
			if err != nil {
				return results, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			err = el.ScrollIntoView()
			if err != nil {
				return results, fmt.Errorf("failed to scroll element %s into view: %w", action.Selector, err)
			}
			log.Info().Str("selector", action.Selector).Msg("Browser action scrolled element into view")

		case ActionScreenshot:
			if action.Selector != "" {
				el, err := page.Element(action.Selector)
				if err != nil {
					return results, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
				}
				data, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90)
				if err != nil {
					return results, fmt.Errorf("failed to take screenshot of element %s: %w", action.Selector, err)
				}

				if action.File != "" {
					err = os.WriteFile(action.File, data, 0644)
					if err != nil {
						return results, fmt.Errorf("failed to save screenshot to file %s: %w", action.File, err)
					}
				}

				encodedData := base64.StdEncoding.EncodeToString(data)

				results.Screenshots = append(results.Screenshots, ScreenshotResult{
					Selector:   action.Selector,
					Data:       encodedData,
					OutputFile: action.File,
				})

				log.Info().
					Str("selector", action.Selector).
					Msg("Browser action took screenshot of element")
			} else {
				data, err := page.Screenshot(true, nil)
				if err != nil {
					return results, fmt.Errorf("failed to take screenshot of page: %w", err)
				}

				if action.File != "" {
					err = os.WriteFile(action.File, data, 0644)
					if err != nil {
						return results, fmt.Errorf("failed to save screenshot to file %s: %w", action.File, err)
					}
				}

				encodedData := base64.StdEncoding.EncodeToString(data)

				results.Screenshots = append(results.Screenshots, ScreenshotResult{
					Selector:   "",
					Data:       encodedData,
					OutputFile: action.File,
				})

				log.Info().
					Msg("Browser action took screenshot of page")
			}

		case ActionSleep:
			select {
			case <-time.After(time.Duration(action.Duration) * time.Millisecond):
			case <-ctx.Done():
				return results, ctx.Err() // Cancel sleep if context is done
			}
			log.Info().Int("duration", action.Duration).Msg("Browser action slept")

		case ActionEvaluate:
			result, err := page.Eval(action.Expression)
			if err != nil {
				return results, fmt.Errorf("error evaluating JavaScript: %w", err)
			}
			log.Info().Str("expression", action.Expression).Interface("result", result).Msg("Browser action evaluated JavaScript")
		}
	}
	return results, nil
}
