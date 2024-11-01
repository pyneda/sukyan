package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var SSOPaths = []string{
	"saml/metadata",
	".well-known/saml-configuration",
	"saml2/metadata",
	"simplesaml/saml2/idp/metadata.php",
	"simplesaml/module.php/saml/sp/metadata.php/default-sp",
	"Shibboleth.sso/Metadata",
	"sso/metadata",
	"auth/saml2/metadata.php",
	"adfs/ls/idpinitiatedsignon",
	"adfs/services/trust/mex",
	"FederationMetadata/2007-06/FederationMetadata.xml",
	"saml/SSO",
	"sso/saml",
	"login/metadata",
	"metadata/saml20",
	"saml/config",
	"auth/saml/metadata",
	"sso/saml/metadata",
	"okta-saml",
	"onelogin-saml",
	"auth/realms/master/protocol/saml/descriptor",
	"azure/saml2",
	"auth0-saml",
}

func isSSOSetupValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	bodyStr := string(history.ResponseBody)
	details := fmt.Sprintf("SSO configuration found: %s\n", history.URL)
	confidence := 0

	samlIndicators := []string{
		"EntityDescriptor",
		"IDPSSODescriptor",
		"SPSSODescriptor",
		"SingleSignOnService",
		"AssertionConsumerService",
		"<md:EntityDescriptor",
		"<saml:",
		"<samlp:",
		"SingleLogoutService",
		"X509Certificate",
	}

	if strings.Contains(history.ResponseContentType, "application/xml") ||
		strings.Contains(history.ResponseContentType, "application/samlmetadata+xml") {
		confidence += 10
		details += "- SAML metadata content type detected\n"
	}

	for _, indicator := range samlIndicators {
		if strings.Contains(bodyStr, indicator) {
			confidence += 10
			details += fmt.Sprintf("- Contains SAML metadata: %s\n", indicator)
		}
	}

	headers, _ := history.GetResponseHeadersAsMap()
	if _, exists := headers["X-SAML"]; exists {
		confidence += 10
		details += "- SAML-related header detected\n"
	}

	if confidence > 100 {
		confidence = 100
	}

	if confidence >= 20 {
		return true, details, confidence
	}

	return false, "", 0
}

func DiscoverSSOEndpoints(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       SSOPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "application/xml,application/samlmetadata+xml",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: isSSOSetupValidationFunc,
		IssueCode:      db.SsoMetadataDetectedCode,
	})
}
