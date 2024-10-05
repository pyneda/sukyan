package browser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"gopkg.in/yaml.v3"
)

type ActionType string
type WaitCondition string
type AssertCondition string
type ScrollPosition string

const (
	ActionNavigate   ActionType = "navigate"
	ActionClick      ActionType = "click"
	ActionFill       ActionType = "fill"
	ActionWait       ActionType = "wait"
	ActionAssert     ActionType = "assert"
	ActionScroll     ActionType = "scroll"
	ActionScreenshot ActionType = "screenshot"
	ActionSleep      ActionType = "sleep"
	ActionEvaluate   ActionType = "evaluate"

	WaitVisible WaitCondition = "visible"
	WaitHidden  WaitCondition = "hidden"
	WaitEnabled WaitCondition = "enabled"
	WaitLoad    WaitCondition = "load"

	AssertContains AssertCondition = "contains"
	AssertEquals   AssertCondition = "equals"
	AssertVisible  AssertCondition = "visible"
	AssertHidden   AssertCondition = "hidden"

	ScrollTop    ScrollPosition = "top"
	ScrollBottom ScrollPosition = "bottom"
)

type Action struct {
	Type       ActionType      `yaml:"type" json:"type"`
	Selector   string          `yaml:"selector,omitempty" json:"selector,omitempty"`
	Value      string          `yaml:"value,omitempty" json:"value,omitempty"`
	URL        string          `yaml:"url,omitempty" json:"url,omitempty"`
	For        WaitCondition   `yaml:"for,omitempty" json:"for,omitempty"`
	Condition  AssertCondition `yaml:"condition,omitempty" json:"condition,omitempty"`
	Position   ScrollPosition  `yaml:"position,omitempty" json:"position,omitempty"`
	File       string          `yaml:"file,omitempty" json:"file,omitempty"`
	Duration   int             `yaml:"duration,omitempty" json:"duration,omitempty"`
	Expression string          `yaml:"expression,omitempty" json:"expression,omitempty"`
}

type BrowserActions struct {
	Title   string   `yaml:"title" json:"title"`
	Actions []Action `yaml:"actions" json:"actions"`
}

func LoadBrowserActions(path string) (BrowserActions, error) {
	var config BrowserActions
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func ExecuteActions(ctx context.Context, page *rod.Page, actions []Action) error {
	for _, action := range actions {
		select {
		case <-ctx.Done():
			return ctx.Err() // Return context cancellation error
		default:
		}

		switch action.Type {
		case ActionNavigate:
			err := page.Navigate(action.URL)
			if err != nil {
				return fmt.Errorf("failed to navigate to %s: %w", action.URL, err)
			}
			err = page.WaitLoad()
			if err != nil {
				return fmt.Errorf("failed to wait for page load: %w", err)
			}

		case ActionWait:
			el, err := page.Element(action.Selector)
			if err != nil {
				return fmt.Errorf("failed to find element %s: %w", action.Selector, err)
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
				return fmt.Errorf("failed to wait for element %s: %w", action.Selector, err)
			}

		case ActionClick:
			el, err := page.Element(action.Selector)
			if err != nil {
				return fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			err = el.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return fmt.Errorf("failed to click element %s: %w", action.Selector, err)
			}

		case ActionFill:
			el, err := page.Element(action.Selector)
			if err != nil {
				return fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			err = el.Input(action.Value)
			if err != nil {
				return fmt.Errorf("failed to fill element %s with value %s: %w", action.Selector, action.Value, err)
			}

		case ActionAssert:
			el, err := page.Element(action.Selector)
			if err != nil {
				return fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			text, err := el.Text()
			if err != nil {
				return fmt.Errorf("failed to get text of element %s: %w", action.Selector, err)
			}
			switch action.Condition {
			case AssertContains:
				if !strings.Contains(text, action.Value) {
					return fmt.Errorf("assertion failed: element text does not contain '%s'", action.Value)
				}
			case AssertEquals:
				if text != action.Value {
					return fmt.Errorf("assertion failed: element text is not equal to '%s'", action.Value)
				}
			case AssertVisible:
				isVisible, err := el.Visible()
				if err != nil {
					return fmt.Errorf("failed to check visibility of element %s: %w", action.Selector, err)
				}
				if !isVisible {
					return fmt.Errorf("assertion failed: element %s is not visible", action.Selector)
				}
			case AssertHidden:
				isVisible, err := el.Visible()
				if err != nil {
					return fmt.Errorf("failed to check visibility of element %s: %w", action.Selector, err)
				}
				if isVisible {
					return fmt.Errorf("assertion failed: element %s is visible, expected hidden", action.Selector)
				}
			}

		case ActionScroll:
			el, err := page.Element(action.Selector)
			if err != nil {
				return fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			err = el.ScrollIntoView()
			if err != nil {
				return fmt.Errorf("failed to scroll element %s into view: %w", action.Selector, err)
			}

		case ActionScreenshot:
			if action.Selector != "" {
				el, err := page.Element(action.Selector)
				if err != nil {
					return fmt.Errorf("failed to find element %s: %w", action.Selector, err)
				}
				data, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90)
				if err != nil {
					return fmt.Errorf("failed to take screenshot of element %s: %w", action.Selector, err)
				}
				err = os.WriteFile(action.File, data, 0644)
				if err != nil {
					return fmt.Errorf("failed to save screenshot to file %s: %w", action.File, err)
				}
			} else {
				data, err := page.Screenshot(true, nil)
				if err != nil {
					return fmt.Errorf("failed to take screenshot of page: %w", err)
				}
				err = os.WriteFile(action.File, data, 0644)
				if err != nil {
					return fmt.Errorf("failed to save screenshot to file %s: %w", action.File, err)
				}
			}

		case ActionSleep:
			select {
			case <-time.After(time.Duration(action.Duration) * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err() // Cancel sleep if context is done
			}

		case ActionEvaluate:
			result, err := page.Eval(action.Expression)
			if err != nil {
				return fmt.Errorf("error evaluating JavaScript: %w", err)
			}
			fmt.Println("Evaluation result:", result.Value)
		}
	}
	return nil
}
