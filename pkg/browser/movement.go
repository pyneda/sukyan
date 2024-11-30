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
	"github.com/go-rod/rod/lib/proto"
)

type EventTypes struct {
	Click    bool
	Hover    bool
	Movement bool
	Drag     bool
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
		MaxDuration:       2 * time.Minute,
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
	if !events.Click && !events.Hover && !events.Movement && !events.Drag {
		return ""
	}

	selectors := make([]string, 0)
	if events.Click {
		selectors = append(selectors, "*[onclick]", "*[ondblclick]")
	}
	if events.Hover {
		selectors = append(selectors, "*[onmouseover]", "*[onmouseenter]", "*[onmouseleave]")
	}
	if events.Movement {
		selectors = append(selectors, "*[onmousemove]")
	}
	if events.Drag {
		selectors = append(selectors, "*[ondrag]", "*[ondragstart]", "*[ondragend]", "*[ondragenter]", "*[ondragleave]", "*[ondragover]")
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

func interactWithElement(page *rod.Page, el *rod.Element, events EventTypes, opts *MovementOptions) error {
	if !IsElementInteractable(el) {
		return errors.New("element is not interactable")
	}

	if err := moveToElement(page, el, opts); err != nil {
		return err
	}

	if events.Hover {
		time.Sleep(opts.HoverDuration)
	}

	if events.Click {
		if err := page.Mouse.Click(proto.InputMouseButtonLeft, 1); err != nil {
			return fmt.Errorf("failed to click: %w", err)
		}
	}

	return nil
}

func TriggerMouseEvents(page *rod.Page, events EventTypes, opts *MovementOptions) error {
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
			return ctx.Err()
		default:
		}

		var lastErr error
		for retry := 0; retry < opts.MaxRetries; retry++ {
			if err := interactWithElement(page, el, events, opts); err != nil {
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
			// Add some random movement between elements
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
