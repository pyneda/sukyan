package discovery

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var PaymentTestPaths = []string{
	// Stripe
	"stripe/test",
	"stripe-webhook",
	"stripe/webhook",
	"payment-intents/test",

	// PayPal
	"paypal/sandbox",
	"paypal/ipn",

	// Generic Payment/Checkout
	"checkout/test",
	"payments/test",
	"payment/sandbox",
	"checkout/sandbox",
	"payment/test",
	"api/payments/test",
	"v1/payments/test",

	// Additional Common Providers
	"square/sandbox",
	"braintree/sandbox",
	"braintree/test",
	"adyen/test",
	"adyen/webhook/test",
	"mollie/test",
	"razorpay/test",
	"cybersource/test",
	"checkout.com/test",

	// Common Webhook Patterns
	"webhooks/payment/test",
	"payment/webhook/test",

	// API Versioned Endpoints
	"api/v1/payments/test",
	"api/v1/checkout/test",

	// Debug/Development
	"payment/debug",
	"payment/sandbox/debug",
}

func isPaymentTestEndpointValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 && history.StatusCode != 401 && history.StatusCode != 403 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("Payment test endpoint found: %s\n", history.URL)
	confidence := 0

	paymentProviders := map[string][]string{
		"Stripe": {
			"stripe",
			"sk_test_",
			"pk_test_",
			"webhook_secret",
			"payment_intent",
			"payment_method",
		},
		"PayPal": {
			"paypal",
			"sandbox",
			"PAYPAL_",
			"ipn_url",
			"client_id",
			"client_secret",
		},
		"Square": {
			"square",
			"sandbox-",
			"SQUARE_",
			"access_token",
			"location_id",
		},
		"Braintree": {
			"braintree",
			"merchant_id",
			"public_key",
			"private_key",
		},
		"Adyen": {
			"adyen",
			"merchant_account",
			"api_key",
			"client_key",
			"hmac_key",
		},
	}

	testCardPatterns := []string{
		"4242424242424242",
		"4111111111111111",
		"5555555555554444",
		"test_card",
		"test-card",
		"sandbox",
		"test_mode",
		"test-mode",
		"test_transaction",
	}

	var jsonData map[string]interface{}
	isJSON := json.Unmarshal([]byte(bodyStr), &jsonData) == nil

	if isJSON {
		confidence += 10
	}

	for provider, indicators := range paymentProviders {
		for _, indicator := range indicators {
			if strings.Contains(strings.ToLower(bodyStr), strings.ToLower(indicator)) {
				confidence += 15
				details += fmt.Sprintf("- Contains %s payment test indicator: %s\n", provider, indicator)
			}
		}
	}

	for _, pattern := range testCardPatterns {
		if strings.Contains(bodyStr, pattern) {
			confidence += 15
			details += fmt.Sprintf("- Contains test card information: %s\n", pattern)
		}
	}

	headers, _ := history.GetResponseHeadersAsMap()
	for header, values := range headers {
		headerValue := strings.Join(values, " ")
		if strings.Contains(strings.ToLower(headerValue), "sandbox") ||
			strings.Contains(strings.ToLower(headerValue), "test") {
			confidence += 10
			details += fmt.Sprintf("- Payment test-related header found: %s\n", header)
		}
	}

	if strings.Contains(strings.ToLower(history.URL), "test") ||
		strings.Contains(strings.ToLower(history.URL), "sandbox") {
		confidence += 10
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 25 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverPaymentTestEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       PaymentTestPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "*/*",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
		},
		ValidationFunc: isPaymentTestEndpointValidationFunc,
		IssueCode:      db.PaymentTestEndpointDetectedCode,
	})
}
