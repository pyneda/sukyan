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
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

func ExecuteActions(ctx context.Context, page *rod.Page, actions []Action) (ActionsExecutionResults, error) {
	var results ActionsExecutionResults
	results.Steps = make([]ActionStepResult, 0, len(actions))
	actionLogger := lib.NewActionLogger()
	runStart := time.Now()

	finish := func(err error) (ActionsExecutionResults, error) {
		results.DurationMs = time.Since(runStart).Milliseconds()
		results.Logs = actionLogger.GetLogs()
		return results, err
	}

	for i, action := range actions {
		select {
		case <-ctx.Done():
			actionLogger.Log(lib.INFO, "context cancelled")
			return finish(ctx.Err())
		default:
		}

		step := ActionStepResult{
			Index:  i,
			Type:   string(action.Type),
			Target: actionTarget(action),
			Status: StepStatusOK,
		}

		start := time.Now()
		out, err := executeAction(ctx, page, action, actionLogger)
		step.DurationMs = time.Since(start).Milliseconds()
		step.Screenshot = out.screenshot
		step.Evaluation = out.evaluation
		step.Assertion = out.assertion

		if out.screenshot != nil {
			results.Screenshots = append(results.Screenshots, *out.screenshot)
		}

		if err != nil {
			step.Status = StepStatusFailed
			step.Error = err.Error()
			results.Steps = append(results.Steps, step)
			results.Failure = &ActionFailure{
				StepIndex: i,
				Type:      string(action.Type),
				Message:   err.Error(),
			}
			for j := i + 1; j < len(actions); j++ {
				results.Steps = append(results.Steps, ActionStepResult{
					Index:  j,
					Type:   string(actions[j].Type),
					Target: actionTarget(actions[j]),
					Status: StepStatusSkipped,
				})
			}
			return finish(err)
		}

		results.Steps = append(results.Steps, step)
	}

	results.Succeeded = true
	actionLogger.Log(lib.INFO, "all actions completed successfully")
	log.Info().Msg("Browser actions completed successfully")
	return finish(nil)
}

// executeAction performs a single action and returns any payload it produced.
// Callers record status, timing and failure; this function only executes.
func executeAction(ctx context.Context, page *rod.Page, action Action, actionLogger *lib.ActionLogger) (stepOutput, error) {
	var out stepOutput

	switch action.Type {
	case ActionNavigate:
		if err := page.Navigate(action.URL); err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to navigate to %s: %s", action.URL, err))
			return out, fmt.Errorf("failed to navigate to %s: %w", action.URL, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("navigated to %s", action.URL))
		if err := page.WaitLoad(); err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to wait for page load: %s", err))
			return out, fmt.Errorf("failed to wait for page load: %w", err)
		}
		actionLogger.Log(lib.INFO, "page loaded")
		log.Info().Str("url", action.URL).Msg("Browser action navigated to URL")

	case ActionWait:
		el, err := page.Element(action.Selector)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to find element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
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
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to wait for element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to wait for element %s: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("waited for element %s", action.Selector))
		log.Info().Str("selector", action.Selector).Msg("Browser action waited for element")

	case ActionClick:
		el, err := page.Element(action.Selector)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to find element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("clicking element %s", action.Selector))
		err = el.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to click element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to click element %s: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("clicked element %s", action.Selector))
		log.Info().Str("selector", action.Selector).Msg("Browser action clicked element")

	case ActionFill:
		el, err := page.Element(action.Selector)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to find element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("filling element %s with value %s", action.Selector, action.Value))
		err = el.Input(action.Value)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to fill element %s with value %s: %s", action.Selector, action.Value, err))
			return out, fmt.Errorf("failed to fill element %s with value %s: %w", action.Selector, action.Value, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("filled element %s with value %s", action.Selector, action.Value))
		log.Info().Str("selector", action.Selector).Str("value", action.Value).Msg("Browser action filled element")

	case ActionAssert:
		el, err := page.Element(action.Selector)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to find element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("asserting element %s", action.Selector))
		text, err := el.Text()
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to get text of element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to get text of element %s: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("got text of element %s: %s", action.Selector, text))
		switch action.Condition {
		case AssertContains:
			if !strings.Contains(text, action.Value) {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("assertion failed: element text does not contain '%s'", action.Value))
				return out, fmt.Errorf("assertion failed: element text does not contain '%s'", action.Value)
			}
			actionLogger.Log(lib.INFO, fmt.Sprintf("assertion passed: element text contains '%s'", action.Value))
			log.Info().Str("selector", action.Selector).Str("value", action.Value).Msg("Browser action contains assertion passed")
		case AssertEquals:
			if text != action.Value {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("assertion failed: element text is not equal to '%s'", action.Value))
				return out, fmt.Errorf("assertion failed: element text is not equal to '%s'", action.Value)
			}
			actionLogger.Log(lib.INFO, fmt.Sprintf("assertion passed: element text is equal to '%s'", action.Value))
			log.Info().Str("selector", action.Selector).Str("value", action.Value).Msg("Browser action equals assertion passed")
		case AssertVisible:
			isVisible, err := el.Visible()
			actionLogger.Log(lib.INFO, fmt.Sprintf("checking visibility of element %s", action.Selector))
			if err != nil {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to check visibility of element %s: %s", action.Selector, err))
				return out, fmt.Errorf("failed to check visibility of element %s: %w", action.Selector, err)
			}
			if !isVisible {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("assertion failed: element %s is not visible", action.Selector))
				return out, fmt.Errorf("assertion failed: element %s is not visible", action.Selector)
			}
			actionLogger.Log(lib.INFO, fmt.Sprintf("assertion passed: element %s is visible", action.Selector))
			log.Info().Str("selector", action.Selector).Msg("Browser action visible assertion passed")

		case AssertHidden:
			isVisible, err := el.Visible()
			actionLogger.Log(lib.INFO, fmt.Sprintf("checking visibility of element %s", action.Selector))
			if err != nil {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to check visibility of element %s: %s", action.Selector, err))
				return out, fmt.Errorf("failed to check visibility of element %s: %w", action.Selector, err)
			}
			if isVisible {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("assertion failed: element %s is visible, expected hidden", action.Selector))
				return out, fmt.Errorf("assertion failed: element %s is visible, expected hidden", action.Selector)
			}
			actionLogger.Log(lib.INFO, fmt.Sprintf("assertion passed: element %s is hidden", action.Selector))
			log.Info().Str("selector", action.Selector).Msg("Browser action hidden assertion passed")
		}

	case ActionScroll:
		el, err := page.Element(action.Selector)
		actionLogger.Log(lib.INFO, fmt.Sprintf("scrolling element %s into view", action.Selector))

		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to find element %s: %s", action.Selector, err))
			return out, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
		}
		err = el.ScrollIntoView()
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to scroll element %s into view: %s", action.Selector, err))
			return out, fmt.Errorf("failed to scroll element %s into view: %w", action.Selector, err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("scrolled element %s into view", action.Selector))
		log.Info().Str("selector", action.Selector).Msg("Browser action scrolled element into view")

	case ActionScreenshot:
		if action.Selector != "" {
			actionLogger.Log(lib.INFO, fmt.Sprintf("taking screenshot of element %s", action.Selector))
			el, err := page.Element(action.Selector)
			if err != nil {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to find element %s: %s", action.Selector, err))
				return out, fmt.Errorf("failed to find element %s: %w", action.Selector, err)
			}
			data, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 90)
			if err != nil {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to take screenshot of element %s: %s", action.Selector, err))
				return out, fmt.Errorf("failed to take screenshot of element %s: %w", action.Selector, err)
			}
			actionLogger.Log(lib.INFO, fmt.Sprintf("took screenshot of element %s", action.Selector))
			if action.File != "" {
				err = os.WriteFile(action.File, data, 0644)
				if err != nil {
					actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to save screenshot to file %s: %s", action.File, err))
					return out, fmt.Errorf("failed to save screenshot to file %s: %w", action.File, err)
				}
				actionLogger.Log(lib.INFO, fmt.Sprintf("saved screenshot to file %s", action.File))
			}

			encodedData := base64.StdEncoding.EncodeToString(data)

			out.screenshot = &ScreenshotResult{Selector: action.Selector, Data: encodedData, OutputFile: action.File}

			log.Info().
				Str("selector", action.Selector).
				Msg("Browser action took screenshot of element")

		} else {
			actionLogger.Log(lib.INFO, "taking screenshot of page")
			data, err := page.Screenshot(true, nil)
			if err != nil {
				actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to take screenshot of page: %s", err))
				return out, fmt.Errorf("failed to take screenshot of page: %w", err)
			}
			actionLogger.Log(lib.INFO, "took screenshot of page")

			if action.File != "" {
				err = os.WriteFile(action.File, data, 0644)
				if err != nil {
					actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to save screenshot to file %s: %s", action.File, err))

					return out, fmt.Errorf("failed to save screenshot to file %s: %w", action.File, err)
				}
				actionLogger.Log(lib.INFO, fmt.Sprintf("saved screenshot to file %s", action.File))
			}

			encodedData := base64.StdEncoding.EncodeToString(data)

			out.screenshot = &ScreenshotResult{Selector: "", Data: encodedData, OutputFile: action.File}

			log.Info().
				Msg("Browser action took screenshot of page")
		}

	case ActionSleep:
		actionLogger.Log(lib.INFO, fmt.Sprintf("sleeping for %d milliseconds", action.Duration))
		select {
		case <-time.After(time.Duration(action.Duration) * time.Millisecond):
		case <-ctx.Done():
			actionLogger.Log(lib.INFO, "sleep cancelled")
			return out, ctx.Err() // Cancel sleep if context is done
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("slept for %d milliseconds", action.Duration))
		log.Info().Int("duration", action.Duration).Msg("Browser action slept")

	case ActionEvaluate:
		actionLogger.Log(lib.INFO, "evaluating JavaScript")
		result, err := page.Eval(action.Expression)
		if err != nil {
			actionLogger.Log(lib.ERROR, fmt.Sprintf("failed to evaluate JavaScript: %s", err))
			return out, fmt.Errorf("error evaluating JavaScript: %w", err)
		}
		actionLogger.Log(lib.INFO, fmt.Sprintf("evaluated JavaScript with result: %v", result.Value.String()))
		log.Info().Str("expression", action.Expression).Interface("result", result).Msg("Browser action evaluated JavaScript")
	}

	return out, nil
}
