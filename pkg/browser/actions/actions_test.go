package actions

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/stretchr/testify/assert"
)

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
		Addr:    ":8080",
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

	page := rodBrowser.MustPage("http://localhost:8080/page1")
	page.MustWaitLoad()

	actions := []Action{
		{
			Type:     ActionClick,
			Selector: "#login-button",
		},
	}

	_, err := ExecuteActions(ctx, page, actions)
	assert.NoError(t, err)
	formElement, err := page.Element("#login-form")
	assert.NoError(t, err)

	isFormVisible, err := formElement.Visible()
	assert.NoError(t, err)
	assert.True(t, isFormVisible, "Login form should be visible after clicking the login button")
}

func TestFormFillAndSubmit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server := startTestServer()
	defer server.Shutdown(context.Background())

	rodBrowser := setupRodBrowser(t, true)
	defer rodBrowser.Close()

	page := rodBrowser.MustPage("http://localhost:8080/page1")
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
			URL:  "http://localhost:8080/page2",
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
