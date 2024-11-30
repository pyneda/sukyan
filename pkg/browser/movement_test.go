package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/stretchr/testify/assert"
)

func startTestServer(htmlContent string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(htmlContent))
	})
	return httptest.NewServer(handler)
}

func TestMouseMovement(t *testing.T) {
	testHTML := createTestHTML()
	server := startTestServer(testHTML)
	defer server.Close()

	b := setupRodBrowser(t, true)
	defer b.MustClose()

	page := b.MustPage(server.URL).MustWaitLoad()
	defer page.MustClose()

	opts := &MovementOptions{
		MinSpeed:          5 * time.Millisecond,
		MaxSpeed:          10 * time.Millisecond,
		HoverDuration:     50 * time.Millisecond,
		UseAcceleration:   true,
		AccelerationCurve: 0.7,
		RandomMovements:   false,
		MaxRetries:        1,
		RecoveryWait:      50 * time.Millisecond,
		MaxDuration:       5 * time.Second,
	}

	events := EventTypes{
		Click: true,
		Hover: true,
	}

	err := TriggerMouseEvents(page, events, opts)
	assert.NoError(t, err)

	// Check click was registered
	clicked := page.MustEval(`() => window.buttonClicked`).Bool()
	assert.True(t, clicked)

	// Check hover was triggered
	hovered := page.MustEval(`() => window.divHovered`).Bool()
	assert.True(t, hovered)
}

func TestInteractableElement(t *testing.T) {
	testHTML := createTestHTML()
	server := startTestServer(testHTML)
	defer server.Close()

	b := setupRodBrowser(t, true)
	defer b.MustClose()

	page := b.MustPage(server.URL).MustWaitLoad()
	defer page.MustClose()

	// Test visible element
	btn := page.MustElement("#testButton")
	assert.True(t, IsElementInteractable(btn))

	// Test invisible element
	page.MustEval(`() => document.getElementById('testButton').style.display = 'none'`)
	assert.False(t, IsElementInteractable(btn))
}

func TestMovementTimeout(t *testing.T) {
	testHTML := createTestHTML()
	server := startTestServer(testHTML)
	defer server.Close()

	b := setupRodBrowser(t, true)
	defer b.MustClose()

	page := b.MustPage(server.URL).MustWaitLoad()
	defer page.MustClose()

	opts := &MovementOptions{
		MaxDuration: 1 * time.Millisecond, // Very short timeout
	}

	events := EventTypes{
		Click: true,
		Hover: true,
	}

	err := TriggerMouseEvents(page, events, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
}

func TestAlertTriggerOnMouseEvents(t *testing.T) {
	testHTML := createXSSTestHTML()
	server := startTestServer(testHTML)
	defer server.Close()

	b := setupRodBrowser(t, true)
	defer b.MustClose()

	page := b.MustPage(server.URL).MustWaitLoad()
	// Channel to receive alert events
	alertTriggered := make(chan string, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Set up alert detection
	go page.EachEvent(func(e *proto.PageJavascriptDialogOpening) (stop bool) {
		alertTriggered <- e.Message
		err := proto.PageHandleJavaScriptDialog{Accept: true}.Call(page)
		assert.NoError(t, err)
		return true
	})()

	opts := &MovementOptions{
		MinSpeed:          50 * time.Millisecond,
		MaxSpeed:          100 * time.Millisecond,
		HoverDuration:     200 * time.Millisecond,
		UseAcceleration:   true,
		AccelerationCurve: 0.7,
		RandomMovements:   false,
		MaxRetries:        1,
		RecoveryWait:      50 * time.Millisecond,
		MaxDuration:       5 * time.Second,
	}

	events := EventTypes{
		Hover: true,
	}

	// Ensure element is present and visible before proceeding
	el := page.MustElement("#xss-test")
	visible, err := el.Visible()
	assert.NoError(t, err)
	assert.True(t, visible, "Test element should be visible")

	// Run mouse movements
	err = TriggerMouseEvents(page, events, opts)
	assert.NoError(t, err)

	// Wait for alert with timeout
	select {
	case alertMsg := <-alertTriggered:
		assert.Equal(t, "XSS Test Alert", alertMsg)
	case <-ctx.Done():
		t.Error("Timeout waiting for alert to trigger")
	}
}

func createXSSTestHTML() string {
	return `<!DOCTYPE html>
		<html>
			<head>
				<style>
					#xss-test {
						width: 100px;
						height: 100px;
						background: blue;
						position: absolute;
						top: 50%;
						left: 50%;
						transform: translate(-50%, -50%);
					}
				</style>
			</head>
			<body>
				<div id="xss-test" onmouseover="alert('XSS Test Alert')">
					Hover for XSS
				</div>
			</body>
		</html>`

}

func createTestHTML() string {
	return `<html>
			<body>
				<button id="testButton" onclick="window.buttonClicked=true">Click me</button>
				<div id="testDiv" onmouseover="window.divHovered=true"
					style="width:100px;height:100px;background:blue;">
					Hover me
				</div>
				<script>
					window.buttonClicked = false;
					window.divHovered = false;
				</script>
			</body>
		</html>`
}
