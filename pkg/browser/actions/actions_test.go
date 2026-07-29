package actions

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/assert"
)

// TestMain ensures the browser is downloaded before running any tests.
// This prevents test timeouts caused by browser download during test execution.
func TestMain(m *testing.M) {
	// Pre-download browser if not already present by launching and closing it
	l := launcher.New().Headless(true).Set("no-sandbox", "true")
	u := l.MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	browser.MustClose()
	os.Exit(m.Run())
}

const testHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Page</title>
</head>
<body>
    <h1 id="welcome-message">Welcome to Test Page</h1>
    <button id="login-button">Login</button>
    <form id="login-form" style="display: none;">
        <input type="text" id="username" placeholder="Enter your username">
        <input type="password" id="password" placeholder="Enter your password">
        <button type="submit">Submit</button>
    </form>
    <script>
        document.getElementById("login-button").addEventListener("click", function() {
            document.getElementById("login-form").style.display = "block";
        });
    </script>
</body>
</html>
`

const testHTML2 = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Page 2</title>
</head>
<body>
    <h1 id="welcome-message">Welcome to Test Page 2</h1>

    <form id="form">
        <input type="text" id="firstName" placeholder="Enter your first name">
        <input type="text" id="lastName" placeholder="Enter your last name">
        <input type="email" id="email" placeholder="Enter your email">
        <button type="button" id="submit-button">Submit</button>
    </form>

    <div id="confirmation-message" style="display: none;"></div>

    <button id="scroll-button">Scroll to Bottom</button>
    <div id="bottom-section" style="margin-top: 1500px;">
        <p>You have reached the bottom of the page.</p>
    </div>

    <script>
        document.getElementById("submit-button").addEventListener("click", function() {
            const firstName = document.getElementById("firstName").value;
            const lastName = document.getElementById("lastName").value;
            const email = document.getElementById("email").value;

            if (firstName && lastName && email) {
                document.getElementById("confirmation-message").textContent = 
                    "Thank you, " + firstName + " " + lastName + ". Your email " + email + " has been submitted.";
                document.getElementById("confirmation-message").style.display = "block";
            } else {
                alert("Please fill in all fields.");
            }
        });

        document.getElementById("scroll-button").addEventListener("click", function() {
            document.getElementById("bottom-section").scrollIntoView({ behavior: "smooth" });
        });
    </script>
</body>
</html>`

func startTestServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testHTML)
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testHTML2)
	})

	server := &http.Server{
		Addr:    ":9999",
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(fmt.Sprintf("Failed to start server: %s", err))
		}
	}()

	return server
}

func setupRodBrowser(t *testing.T, headless bool) *rod.Browser {
	t.Helper()
	url := launcher.New().Headless(headless).Set("no-sandbox", "true").MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	return browser
}

func TestClickAndVisibility(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	actions := []Action{
		{
			Type:     ActionClick,
			Selector: "#login-button",
		},
	}

	results, err := ExecuteActions(ctx, page, actions)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(results.Logs), "There should be 3 logs")
	assert.Equal(t, true, results.Succeeded, "The action should have succeeded")
	assert.Equal(t, 0, len(results.Screenshots), "There should be no screenshots")

}

func TestExecuteActionsRecordsSteps(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	acts := []Action{
		{Type: ActionClick, Selector: "#login-button"},
		{Type: ActionSleep, Duration: 50},
	}

	results, err := ExecuteActions(ctx, page, acts)
	assert.NoError(t, err)

	assert.Equal(t, 2, len(results.Steps), "one step per action")
	assert.Equal(t, true, results.Succeeded, "all steps ok means succeeded")
	assert.Nil(t, results.Failure, "no failure on a clean run")

	assert.Equal(t, 0, results.Steps[0].Index)
	assert.Equal(t, string(ActionClick), results.Steps[0].Type)
	assert.Equal(t, "#login-button", results.Steps[0].Target)
	assert.Equal(t, StepStatusOK, results.Steps[0].Status)

	assert.Equal(t, 1, results.Steps[1].Index)
	assert.Equal(t, string(ActionSleep), results.Steps[1].Type)
	assert.GreaterOrEqual(t, results.Steps[1].DurationMs, int64(50), "sleep duration is measured")
	assert.GreaterOrEqual(t, results.DurationMs, results.Steps[1].DurationMs)
}

// TestExecuteActionsCancelledBetweenActions covers cancellation observed at
// the top-of-loop ctx.Done() check - the gap between actions - as opposed to
// cancellation observed mid-action (e.g. inside ActionSleep's own select,
// which was already correct before this fix). The context is cancelled
// before ExecuteActions is even called: context.WithCancel closes its Done
// channel synchronously inside cancel(), so this is fully deterministic (no
// timer/goroutine race) and guarantees the very first top-of-loop check
// observes the context as already done, before action 0 ever runs. The loop
// branch under test does not treat index 0 specially - the same code path
// handles cancellation at any index - so this deterministically exercises
// exactly the fixed logic: every action must still get a step, all of them
// skipped, with Failure populated for the action that didn't get to run.
func TestExecuteActionsCancelledBetweenActions(t *testing.T) {
	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	acts := []Action{
		{Type: ActionClick, Selector: "#login-button"},
		{Type: ActionFill, Selector: "#username", Value: "testuser"},
		{Type: ActionFill, Selector: "#password", Value: "testpassword"},
	}

	results, err := ExecuteActions(ctx, page, acts)
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, results.Succeeded)

	assert.Equal(t, len(acts), len(results.Steps), "every action gets a step, even ones that never ran")
	for i, step := range results.Steps {
		assert.Equal(t, i, step.Index)
		assert.Equal(t, string(acts[i].Type), step.Type)
		assert.Equal(t, StepStatusSkipped, step.Status, "cancellation before it ran means the action is skipped")
	}

	if assert.NotNil(t, results.Failure, "cancellation between actions must still populate Failure") {
		assert.Equal(t, 0, results.Failure.StepIndex)
		assert.Equal(t, string(ActionClick), results.Failure.Type)
		assert.Equal(t, context.Canceled.Error(), results.Failure.Message)
	}
}

func TestFormFillAndSubmit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	actions := []Action{
		{
			Type:     ActionClick,
			Selector: "#login-button",
		},
		{
			Type:     ActionFill,
			Selector: "#username",
			Value:    "testuser",
		},
		{
			Type:     ActionFill,
			Selector: "#password",
			Value:    "testpassword",
		},
	}

	_, err := ExecuteActions(ctx, page, actions)
	assert.NoError(t, err)

	// Verify that the username field is filled correctly
	usernameElement, err := page.Element("#username")
	assert.NoError(t, err)

	usernameValue, err := usernameElement.Property("value")
	assert.NoError(t, err)
	assert.Equal(t, "testuser", usernameValue.String(), "Username should be 'testuser'")

	// Verify that the password field is filled correctly
	passwordElement, err := page.Element("#password") // Get both the element and error
	assert.NoError(t, err)

	passwordValue, err := passwordElement.Property("value")
	assert.NoError(t, err)
	assert.Equal(t, "testpassword", passwordValue.String(), "Password should be 'testpassword'")
}

func TestFormFillAndScrollOnPage2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()
	page := rodBrowser.MustPage("")

	actions := []Action{
		{
			Type: ActionNavigate,
			URL:  "http://localhost:9999/page2",
		},
		{
			Type:     ActionFill,
			Selector: "#firstName",
			Value:    "John",
		},
		{
			Type:     ActionFill,
			Selector: "#lastName",
			Value:    "Doe",
		},
		{
			Type:     ActionFill,
			Selector: "#email",
			Value:    "john.doe@example.com",
		},
		{
			Type:     ActionClick,
			Selector: "#submit-button",
		},
		{
			Type:     ActionClick,
			Selector: "#scroll-button",
		},
	}

	_, err := ExecuteActions(ctx, page, actions)
	assert.NoError(t, err)

	// Verify that the first name field is filled correctly
	firstNameElement, err := page.Element("#firstName")
	assert.NoError(t, err)

	firstNameValue, err := firstNameElement.Property("value")
	assert.NoError(t, err)
	assert.Equal(t, "John", firstNameValue.String(), "First name should be 'John'")

	// Verify that the last name field is filled correctly
	lastNameElement, err := page.Element("#lastName")
	assert.NoError(t, err)

	lastNameValue, err := lastNameElement.Property("value")
	assert.NoError(t, err)
	assert.Equal(t, "Doe", lastNameValue.String(), "Last name should be 'Doe'")

	// Verify that the email field is filled correctly
	emailElement, err := page.Element("#email")
	assert.NoError(t, err)

	emailValue, err := emailElement.Property("value")
	assert.NoError(t, err)
	assert.Equal(t, "john.doe@example.com", emailValue.String(), "Email should be 'john.doe@example.com'")

	// Verify that the confirmation message is displayed and contains the correct text
	confirmationElement, err := page.Element("#confirmation-message")
	assert.NoError(t, err)

	confirmationText, err := confirmationElement.Text()
	assert.NoError(t, err)
	assert.Contains(t, confirmationText, "Thank you, John Doe", "Confirmation message should contain 'Thank you, John Doe'")

	// Verify that the scroll to the bottom works
	bottomSectionElement, err := page.Element("#bottom-section")
	assert.NoError(t, err)

	isBottomVisible, err := bottomSectionElement.Visible()
	assert.NoError(t, err)
	assert.True(t, isBottomVisible, "Bottom section should be visible after scrolling")
}

func TestExecuteActionsCapturesEvaluationValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	acts := []Action{
		{Type: ActionEvaluate, Expression: `() => document.title`},
		{Type: ActionEvaluate, Expression: `() => [1, 2, 3]`},
		{Type: ActionEvaluate, Expression: `() => ({a: 1})`},
		{Type: ActionEvaluate, Expression: `() => 42`},
	}

	results, err := ExecuteActions(ctx, page, acts)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(results.Steps))

	strEval := results.Steps[0].Evaluation
	assert.NotNil(t, strEval, "evaluate step carries an evaluation payload")
	assert.Equal(t, `() => document.title`, strEval.Expression)
	assert.Equal(t, "string", strEval.Type)
	assert.JSONEq(t, `"Test Page"`, string(strEval.Value))

	arrEval := results.Steps[1].Evaluation
	assert.NotNil(t, arrEval)
	assert.Equal(t, "array", arrEval.Type, "type is derived from the serialized value, not Chrome's RemoteObject.Subtype")
	assert.JSONEq(t, `[1,2,3]`, string(arrEval.Value))

	objEval := results.Steps[2].Evaluation
	assert.NotNil(t, objEval)
	assert.Equal(t, "object", objEval.Type)
	assert.JSONEq(t, `{"a":1}`, string(objEval.Value))

	numEval := results.Steps[3].Evaluation
	assert.NotNil(t, numEval)
	assert.Equal(t, "number", numEval.Type)
	assert.JSONEq(t, `42`, string(numEval.Value))
}

func TestExecuteActionsRecordsAssertionVerdicts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	passing, err := ExecuteActions(ctx, page, []Action{
		{Type: ActionAssert, Selector: "#welcome-message", Condition: AssertContains, Value: "Welcome"},
	})
	assert.NoError(t, err)
	verdict := passing.Steps[0].Assertion
	assert.NotNil(t, verdict)
	assert.True(t, verdict.Passed)
	assert.Equal(t, "contains", verdict.Condition)
	assert.Equal(t, "#welcome-message", verdict.Selector)
	assert.Equal(t, "Welcome", verdict.Expected)
	assert.Equal(t, "Welcome to Test Page", verdict.Actual)

	failing, err := ExecuteActions(ctx, page, []Action{
		{Type: ActionAssert, Selector: "#welcome-message", Condition: AssertEquals, Value: "Goodbye"},
	})
	assert.Error(t, err, "a failed assertion still ends the run")
	failedVerdict := failing.Steps[0].Assertion
	assert.NotNil(t, failedVerdict, "the verdict survives the failure")
	assert.False(t, failedVerdict.Passed)
	assert.Equal(t, "Goodbye", failedVerdict.Expected)
	assert.Equal(t, "Welcome to Test Page", failedVerdict.Actual)
	assert.Equal(t, StepStatusFailed, failing.Steps[0].Status)
}

func TestExecuteActionsRecordsVisibilityAssertionVerdicts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	visiblePassing, err := ExecuteActions(ctx, page, []Action{
		{Type: ActionAssert, Selector: "#welcome-message", Condition: AssertVisible},
	})
	assert.NoError(t, err)
	visibleVerdict := visiblePassing.Steps[0].Assertion
	assert.NotNil(t, visibleVerdict)
	assert.True(t, visibleVerdict.Passed)
	assert.Equal(t, "visible", visibleVerdict.Expected)
	assert.Equal(t, "visible", visibleVerdict.Actual)

	hiddenPassing, err := ExecuteActions(ctx, page, []Action{
		{Type: ActionAssert, Selector: "#login-form", Condition: AssertHidden},
	})
	assert.NoError(t, err)
	hiddenVerdict := hiddenPassing.Steps[0].Assertion
	assert.NotNil(t, hiddenVerdict)
	assert.True(t, hiddenVerdict.Passed)
	assert.Equal(t, "hidden", hiddenVerdict.Expected)
	assert.Equal(t, "hidden", hiddenVerdict.Actual)

	visibleFailing, err := ExecuteActions(ctx, page, []Action{
		{Type: ActionAssert, Selector: "#login-form", Condition: AssertVisible},
	})
	assert.Error(t, err, "a failed visibility assertion still ends the run")
	failedVerdict := visibleFailing.Steps[0].Assertion
	assert.NotNil(t, failedVerdict, "the verdict survives the failure")
	assert.False(t, failedVerdict.Passed)
	assert.Equal(t, "visible", failedVerdict.Expected)
	assert.Equal(t, "hidden", failedVerdict.Actual)
	assert.Equal(t, StepStatusFailed, visibleFailing.Steps[0].Status)
}

func TestExecuteActionsAssertionNotEvaluatedLeavesNoVerdict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	// A selector that never appears makes rod's element lookup retry until
	// its own context is done. Bound that lookup tightly so this test fails
	// fast instead of hanging (page.Element does not honour the ctx passed
	// to ExecuteActions).
	shortPage := page.Timeout(500 * time.Millisecond)

	results, err := ExecuteActions(ctx, shortPage, []Action{
		{Type: ActionAssert, Selector: "#does-not-exist", Condition: AssertVisible},
	})
	assert.Error(t, err, "failing to find the element still ends the run")
	assert.Len(t, results.Steps, 1)
	step := results.Steps[0]
	assert.Equal(t, StepStatusFailed, step.Status)
	assert.NotEmpty(t, step.Error)
	assert.Nil(t, step.Assertion, "no comparison ever ran, so no verdict should be recorded")
}

func TestExecuteActionsReportsFailureAndSkipsRemaining(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:9999/page1")
	page.MustWaitLoad()

	acts := []Action{
		{Type: ActionClick, Selector: "#login-button"},
		{Type: ActionAssert, Selector: "#welcome-message", Condition: AssertEquals, Value: "Nope"},
		{Type: ActionSleep, Duration: 10},
		{Type: ActionSleep, Duration: 10},
	}

	results, err := ExecuteActions(ctx, page, acts)
	assert.Error(t, err)

	assert.False(t, results.Succeeded)
	assert.NotNil(t, results.Failure)
	assert.Equal(t, 1, results.Failure.StepIndex)
	assert.Equal(t, string(ActionAssert), results.Failure.Type)
	assert.Contains(t, results.Failure.Message, "assertion failed")

	assert.Equal(t, 4, len(results.Steps), "every action gets a step, even after the failure")
	assert.Equal(t, StepStatusOK, results.Steps[0].Status)
	assert.Equal(t, StepStatusFailed, results.Steps[1].Status)
	assert.Equal(t, StepStatusSkipped, results.Steps[2].Status)
	assert.Equal(t, StepStatusSkipped, results.Steps[3].Status)

	assert.NotEmpty(t, results.Logs, "logs collected before the failure survive")
}
