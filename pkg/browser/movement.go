package browser

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type EventTypes struct {
	Click    bool
	Hover    bool
	Movement bool
	Drag     bool
	Focus    bool // onfocus, onblur
	Keyboard bool // onkeydown, onkeyup, onkeypress
	Scroll   bool // onscroll
	Change   bool // onchange, oninput
}

func (e EventTypes) HasEventTypesToCheck() bool {
	return e.Click || e.Hover || e.Movement || e.Drag || e.Focus || e.Keyboard || e.Scroll || e.Change
}

func EventTypesForAlertPayload(payload string) EventTypes {
	events := EventTypes{}

	// Click events
	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onclick", "ondblclick", "onmousedown", "onmouseup"}) {
		events.Click = true
	}

	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onmouseover", "onmouseenter", "onmouseleave", "onmouseout"}) {
		events.Hover = true
	}
	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onmousemove"}) {
		events.Movement = true
	}

	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"ondrag", "ondragstart", "ondragend", "ondragenter", "ondragleave", "ondragover", "ondrop"}) {
		events.Drag = true
	}

	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onfocus", "onblur", "onfocusin", "onfocusout"}) {
		events.Focus = true
	}

	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onkeydown", "onkeyup", "onkeypress"}) {
		events.Keyboard = true
	}

	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onscroll"}) {
		events.Scroll = true
	}

	if lib.ContainsAnySubstringIgnoreCase(payload, []string{"onchange", "oninput"}) {
		events.Change = true
	}

	return events
}

type MovementOptions struct {
	MinSpeed          time.Duration
	MaxSpeed          time.Duration
	HoverDuration     time.Duration
	UseAcceleration   bool
	AccelerationCurve float64
	RandomMovements   bool
	MaxRetries        int
	RecoveryWait      time.Duration
	MaxDuration       time.Duration
	ActionTimeout     time.Duration
}

var (
	DefaultMovementOptions = MovementOptions{
		MinSpeed:          10 * time.Millisecond,
		MaxSpeed:          30 * time.Millisecond,
		HoverDuration:     200 * time.Millisecond,
		UseAcceleration:   true,
		AccelerationCurve: 0.7,
		RandomMovements:   true,
		MaxRetries:        3,
		RecoveryWait:      500 * time.Millisecond,
		MaxDuration:       30 * time.Second,
		ActionTimeout:     5 * time.Second,
	}

	FastMovementOptions = MovementOptions{
		MinSpeed:        5 * time.Millisecond,
		MaxSpeed:        15 * time.Millisecond,
		HoverDuration:   100 * time.Millisecond,
		UseAcceleration: false,
		RandomMovements: false,
		MaxRetries:      2,
		RecoveryWait:    200 * time.Millisecond,
		MaxDuration:     1 * time.Minute,
		ActionTimeout:   3 * time.Second,
	}

	ThoroughMovementOptions = MovementOptions{
		MinSpeed:          20 * time.Millisecond,
		MaxSpeed:          50 * time.Millisecond,
		HoverDuration:     500 * time.Millisecond,
		UseAcceleration:   true,
		AccelerationCurve: 0.8,
		RandomMovements:   true,
		MaxRetries:        5,
		RecoveryWait:      1 * time.Second,
		MaxDuration:       5 * time.Minute,
		ActionTimeout:     10 * time.Second,
	}

	DefaultEventTypes = EventTypes{
		Click:    true,
		Hover:    true,
		Movement: true,
		Drag:     false,
	}
)

type point struct {
	x, y float64
}

func buildEventSelector(events EventTypes) string {
	if !events.HasEventTypesToCheck() {
		return ""
	}

	selectors := make([]string, 0)
	if events.Click {
		selectors = append(selectors, "*[onclick]", "*[ondblclick]", "*[onmousedown]", "*[onmouseup]")
	}
	if events.Hover {
		selectors = append(selectors, "*[onmouseover]", "*[onmouseenter]", "*[onmouseleave]", "*[onmouseout]")
	}
	if events.Movement {
		selectors = append(selectors, "*[onmousemove]")
	}
	if events.Drag {
		selectors = append(selectors, "*[ondrag]", "*[ondragstart]", "*[ondragend]", "*[ondragenter]", "*[ondragleave]", "*[ondragover]", "*[ondrop]")
	}
	if events.Focus {
		selectors = append(selectors, "*[onfocus]", "*[onblur]", "*[onfocusin]", "*[onfocusout]")
	}
	if events.Keyboard {
		selectors = append(selectors, "*[onkeydown]", "*[onkeyup]", "*[onkeypress]")
	}
	if events.Scroll {
		selectors = append(selectors, "*[onscroll]")
	}
	if events.Change {
		selectors = append(selectors, "*[onchange]", "*[oninput]")
	}
	return strings.Join(selectors, ",")
}
func GetElementsWithEvents(page *rod.Page, events EventTypes) ([]*rod.Element, error) {
	selector := buildEventSelector(events)
	elements, err := page.Elements(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to get elements: %w", err)
	}
	return elements, nil
}

func bezierCurve(t float64, start, control1, control2, end point) point {
	t2 := t * t
	t3 := t2 * t
	mt := 1 - t
	mt2 := mt * mt
	mt3 := mt2 * mt

	return point{
		x: mt3*start.x + 3*mt2*t*control1.x + 3*mt*t2*control2.x + t3*end.x,
		y: mt3*start.y + 3*mt2*t*control1.y + 3*mt*t2*control2.y + t3*end.y,
	}
}

func IsElementInteractable(el *rod.Element) bool {
	if el == nil {
		return false
	}

	// Check visibility
	visible, err := el.Visible()
	if err != nil || !visible {
		return false
	}

	// Get element shape and check if it has size
	shape, err := el.Shape()
	if err != nil || shape == nil || len(shape.Quads) == 0 {
		return false
	}

	// Get the first quad which represents the element's box
	quad := shape.Quads[0]
	if len(quad) < 8 { // quads should have 4 points (x,y) = 8 values
		return false
	}

	// Calculate width and height from the quad
	width := quad[2] - quad[0]  // x2 - x1
	height := quad[5] - quad[1] // y3 - y1

	return width > 0 && height > 0
}

func moveToElement(page *rod.Page, el *rod.Element, opts *MovementOptions) error {
	shape, err := el.Shape()
	if err != nil {
		return fmt.Errorf("failed to get element shape: %w", err)
	}

	if len(shape.Quads) == 0 {
		return fmt.Errorf("element has no quads")
	}

	// Get the first quad which represents the element's box
	quad := shape.Quads[0]
	if len(quad) < 8 {
		return fmt.Errorf("invalid quad data")
	}

	// Calculate center point from the quad
	centerX := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	centerY := (quad[1] + quad[3] + quad[5] + quad[7]) / 4

	// Get current mouse position
	pos := page.Mouse.Position()
	currentPos := point{x: pos.X, y: pos.Y}
	targetPos := point{x: centerX, y: centerY}

	// Generate control points for bezier curve
	distance := math.Sqrt(math.Pow(targetPos.x-currentPos.x, 2) + math.Pow(targetPos.y-currentPos.y, 2))
	offset := distance * 0.4
	control1 := point{
		x: currentPos.x + rand.Float64()*offset,
		y: currentPos.y + rand.Float64()*offset,
	}
	control2 := point{
		x: targetPos.x - rand.Float64()*offset,
		y: targetPos.y - rand.Float64()*offset,
	}

	steps := 20 + rand.Intn(10)
	for step := 0; step <= steps; step++ {
		t := float64(step) / float64(steps)
		if opts.UseAcceleration {
			t = math.Pow(t, opts.AccelerationCurve)
		}

		pos := bezierCurve(t, currentPos, control1, control2, targetPos)
		err := page.Mouse.MoveTo(proto.NewPoint(pos.x, pos.y))
		if err != nil {
			return fmt.Errorf("failed to move mouse: %w", err)
		}

		sleepTime := opts.MinSpeed + time.Duration(rand.Int63n(int64(opts.MaxSpeed-opts.MinSpeed)))
		time.Sleep(sleepTime)
		if opts.UseAcceleration {
			jitter := rand.Float64()*2 - 1
			pos.x += jitter
			pos.y += jitter
		}
	}

	return nil
}

func interactWithElement(page *rod.Page, el *rod.Element, events EventTypes, opts *MovementOptions, ctx context.Context) error {
	if !IsElementInteractable(el) {
		return errors.New("element is not interactable")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := moveToElement(page, el, opts); err != nil {
		return err
	}

	if events.Hover {
		timer := time.NewTimer(opts.HoverDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	if events.Click {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := page.Mouse.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return fmt.Errorf("failed to click: %w", err)
		}
	}

	return nil
}

func TriggerMouseEvents(page *rod.Page, events EventTypes, opts *MovementOptions, done <-chan struct{}) error {
	if opts == nil {
		opts = &DefaultMovementOptions
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.MaxDuration)
	defer cancel()

	elements, err := GetElementsWithEvents(page, events)
	if err != nil {
		return err
	}

	for _, el := range elements {
		select {
		case <-ctx.Done():
			log.Info().Str("action", "triggerMouseEvents").Msg("Context done")
			return ctx.Err()
		case <-done:
			return nil
		default:
		}

		var lastErr error
		for retry := 0; retry < opts.MaxRetries; retry++ {
			select {
			case <-ctx.Done():
				log.Info().Str("action", "triggerMouseEvents").Msg("Context done")
				return ctx.Err()
			case <-done:
				return nil
			default:
			}

			actionTimeout := opts.MaxDuration / time.Duration(4)
			if opts.ActionTimeout > 0 {
				actionTimeout = opts.ActionTimeout
			}
			log.Info().Str("action", "interactWithElement").Msg("Interacting with element")
			eleCtx, eleCancel := context.WithTimeout(ctx, actionTimeout)
			err := interactWithElement(page, el, events, opts, eleCtx)
			eleCancel()
			log.Info().Str("action", "interactWithElement").Msg("Finished interacting with element")

			if err != nil {
				lastErr = err
				time.Sleep(opts.RecoveryWait)
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return fmt.Errorf("failed to interact with element after %d retries: %w", opts.MaxRetries, lastErr)
		}

		if opts.RandomMovements && rand.Float64() < 0.3 {
			select {
			case <-ctx.Done():
				log.Info().Str("action", "randomMovement").Msg("Context done")
				return ctx.Err()
			case <-done:
				return nil
			default:
			}

			pos := page.Mouse.Position()
			offsetX := rand.Float64()*40 - 20
			offsetY := rand.Float64()*40 - 20

			if err := page.Mouse.MoveTo(proto.NewPoint(pos.X+offsetX, pos.Y+offsetY)); err != nil {
				return fmt.Errorf("failed random movement: %w", err)
			}
		}
	}

	return nil
}

// TriggerFocusEvents triggers focus/blur events on elements with focus handlers
func TriggerFocusEvents(page *rod.Page, opts *MovementOptions, done <-chan struct{}) error {
	if opts == nil {
		opts = &DefaultMovementOptions
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.MaxDuration)
	defer cancel()

	focusEvents := EventTypes{Focus: true}
	elements, err := GetElementsWithEvents(page, focusEvents)
	if err != nil {
		return err
	}

	for _, el := range elements {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		default:
		}

		if !IsElementInteractable(el) {
			continue
		}

		// Move to element first
		if err := moveToElement(page, el, opts); err != nil {
			log.Debug().Err(err).Msg("Failed to move to element for focus")
			continue
		}

		// Focus the element
		err := el.Focus()
		if err != nil {
			log.Debug().Err(err).Msg("Failed to focus element")
			continue
		}

		// Wait a bit
		time.Sleep(100 * time.Millisecond)

		// Blur the element
		_, err = el.Eval(`() => this.blur()`)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to blur element")
		}
	}

	return nil
}

// TriggerKeyboardEvents triggers keyboard events on elements with keyboard handlers
func TriggerKeyboardEvents(page *rod.Page, opts *MovementOptions, done <-chan struct{}) error {
	if opts == nil {
		opts = &DefaultMovementOptions
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.MaxDuration)
	defer cancel()

	keyboardEvents := EventTypes{Keyboard: true}
	elements, err := GetElementsWithEvents(page, keyboardEvents)
	if err != nil {
		return err
	}

	for _, el := range elements {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		default:
		}

		if !IsElementInteractable(el) {
			continue
		}

		// Focus element first to receive keyboard events
		err := el.Focus()
		if err != nil {
			log.Debug().Err(err).Msg("Failed to focus element for keyboard events")
			continue
		}

		// Send some key presses
		testKeys := []input.Key{input.KeyA, input.KeyB, input.KeyC}
		for _, key := range testKeys {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-done:
				return nil
			default:
			}

			err := page.Keyboard.Type(key)
			if err != nil {
				log.Debug().Err(err).Msg("Failed to type key")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	return nil
}

// TriggerScrollEvents triggers scroll events on elements with scroll handlers
func TriggerScrollEvents(page *rod.Page, opts *MovementOptions, done <-chan struct{}) error {
	if opts == nil {
		opts = &DefaultMovementOptions
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.MaxDuration)
	defer cancel()

	scrollEvents := EventTypes{Scroll: true}
	elements, err := GetElementsWithEvents(page, scrollEvents)
	if err != nil {
		return err
	}

	for _, el := range elements {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		default:
		}

		if !IsElementInteractable(el) {
			continue
		}

		// Move to element
		if err := moveToElement(page, el, opts); err != nil {
			log.Debug().Err(err).Msg("Failed to move to element for scroll")
			continue
		}

		// Scroll the element using JavaScript
		_, err := el.Eval(`() => {
			this.scrollTop += 100;
			this.dispatchEvent(new Event('scroll'));
		}`)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to scroll element")
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Also scroll the page itself
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	default:
	}

	_, err = page.Eval(`() => {
		window.scrollBy(0, 100);
	}`)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to scroll page")
	}

	return nil
}

// TriggerChangeEvents triggers change/input events on form elements
func TriggerChangeEvents(page *rod.Page, opts *MovementOptions, done <-chan struct{}) error {
	if opts == nil {
		opts = &DefaultMovementOptions
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.MaxDuration)
	defer cancel()

	changeEvents := EventTypes{Change: true}
	elements, err := GetElementsWithEvents(page, changeEvents)
	if err != nil {
		return err
	}

	for _, el := range elements {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		default:
		}

		if !IsElementInteractable(el) {
			continue
		}

		// Focus element
		err := el.Focus()
		if err != nil {
			log.Debug().Err(err).Msg("Failed to focus element for change events")
			continue
		}

		// Get element tag name to determine how to interact
		tagName, err := el.Eval(`() => this.tagName.toLowerCase()`)
		if err != nil {
			continue
		}

		tag := tagName.Value.Str()
		switch tag {
		case "input", "textarea":
			// Type some text
			err = el.Input("test")
			if err != nil {
				log.Debug().Err(err).Msg("Failed to input text")
			}
		case "select":
			// Try to change selection
			_, err = el.Eval(`() => {
				if (this.options.length > 1) {
					this.selectedIndex = (this.selectedIndex + 1) % this.options.length;
					this.dispatchEvent(new Event('change'));
				}
			}`)
			if err != nil {
				log.Debug().Err(err).Msg("Failed to change select")
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// TriggerAllEvents is a convenience function that triggers all event types
func TriggerAllEvents(page *rod.Page, events EventTypes, opts *MovementOptions, done <-chan struct{}) error {
	if opts == nil {
		opts = &DefaultMovementOptions
	}

	// Mouse events (existing)
	if events.Click || events.Hover || events.Movement || events.Drag {
		if err := TriggerMouseEvents(page, events, opts, done); err != nil {
			log.Debug().Err(err).Msg("Error triggering mouse events")
		}
	}

	// Focus events
	if events.Focus {
		if err := TriggerFocusEvents(page, opts, done); err != nil {
			log.Debug().Err(err).Msg("Error triggering focus events")
		}
	}

	// Keyboard events
	if events.Keyboard {
		if err := TriggerKeyboardEvents(page, opts, done); err != nil {
			log.Debug().Err(err).Msg("Error triggering keyboard events")
		}
	}

	// Scroll events
	if events.Scroll {
		if err := TriggerScrollEvents(page, opts, done); err != nil {
			log.Debug().Err(err).Msg("Error triggering scroll events")
		}
	}

	// Change events
	if events.Change {
		if err := TriggerChangeEvents(page, opts, done); err != nil {
			log.Debug().Err(err).Msg("Error triggering change events")
		}
	}

	return nil
}
