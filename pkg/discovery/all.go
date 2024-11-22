package discovery

import (
	"fmt"
	"net/http"

	"github.com/pyneda/sukyan/pkg/http_utils"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type DiscoveryResult struct {
	Source  string
	Results DiscoverAndCreateIssueResults
}

type DiscoveryOptions struct {
	BaseURL                string                            `json:"base_url"`
	HistoryCreationOptions http_utils.HistoryCreationOptions `json:"history_creation_options"`
	HttpClient             *http.Client                      `json:"-"`
	SiteBehavior           *http_utils.SiteBehavior          `json:"site_behavior"`
	BaseHeaders            map[string][]string               `json:"base_headers" validate:"omitempty"`
	ScanMode               scan_options.ScanMode             `json:"scan_mode" validate:"omitempty,oneof=fast smart fuzz"`
}

func DiscoverAll(options DiscoveryOptions) ([]DiscoveryResult, error) {
	var allResults []DiscoveryResult
	var errors []error

	if options.HttpClient == nil {
		transport := http_utils.CreateHttpTransport()
		transport.ForceAttemptHTTP2 = true
		options.HttpClient = &http.Client{
			Transport: transport,
		}
	}

	discoveryFunctions := map[string]func(DiscoveryOptions) (DiscoverAndCreateIssueResults, error){
		"openapi":                  DiscoverOpenapiDefinitions,
		"graphql":                  DiscoverGraphQLEndpoints,
		"kubernetes":               DiscoverKubernetesEndpoints,
		"wordpress":                DiscoverWordPressEndpoints,
		"config_files":             DiscoverSensitiveConfigFiles,
		"actuator":                 DiscoverActuatorEndpoints,
		"metrics":                  DiscoverMetricsEndpoints,
		"docker":                   DiscoverDockerAPIEndpoints,
		"vcs":                      DiscoverVersionControlFiles,
		"cicd":                     DiscoverCICDBuildFiles,
		"admin":                    DiscoverAdminInterfaces,
		"db_management":            DiscoverDBManagementInterfaces,
		"cloud_metadata":           DiscoverCloudMetadata,
		"sso":                      DiscoverSSOEndpoints,
		"oauth":                    DiscoverOAuthEndpoints,
		"phpinfo":                  DiscoverPHPInfo,
		"payment_test":             DiscoverPaymentTestEndpoints,
		"socketio":                 DiscoverSocketIO,
		"server_info":              DiscoverServerInfo,
		"logs":                     DiscoverLogFiles,
		"jmx":                      DiscoverHTTPJMXEndpoints,
		"axis2":                    DiscoverAxis2Endpoints,
		"grpc":                     DiscoverGRPCEndpoints,
		"wsdl":                     DiscoverWSDLDefinitions,
		"trace_axd":                DiscoverAspNetTrace,
		"htaccess":                 DiscoverWebServerControlFiles,
		"env_files":                DiscoverEnvFiles,
		"elmah":                    DiscoverElmah,
		"tomcat_uri_normalization": DiscoverTomcatUriNormalization,
		"tomcat_examples":          DiscoverTomcatExamples,
		"crossdomain":              DiscoverFlashCrossDomainPolicy,
		"jboss_invoker":            DiscoverJBossInvokers,
		"jboss_console":            DiscoverJBossConsoles,
		"jboss_status":             DiscoverJBossStatus,
	}

	p := pool.NewWithResults[struct {
		Result DiscoveryResult
		Error  error
	}]().WithMaxGoroutines(5)

	for source, discoverFunc := range discoveryFunctions {
		source := source
		discoverFunc := discoverFunc

		p.Go(func() struct {
			Result DiscoveryResult
			Error  error
		} {
			log.Info().Str("check", source).Str("url", options.BaseURL).Msg("Discovering hidden paths")
			results, err := discoverFunc(options)
			return struct {
				Result DiscoveryResult
				Error  error
			}{
				Result: DiscoveryResult{
					Source:  source,
					Results: results,
				},
				Error: err,
			}
		})
	}

	responses := p.Wait()

	for _, resp := range responses {
		if resp.Error != nil {
			errors = append(errors, fmt.Errorf("%s discovery failed: %w", resp.Result.Source, resp.Error))
		} else {
			allResults = append(allResults, resp.Result)
		}
	}

	if len(errors) > 0 {
		return allResults, fmt.Errorf("some discoveries failed: %v", errors)
	}

	return allResults, nil
}
