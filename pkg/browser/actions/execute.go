package actions

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

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
