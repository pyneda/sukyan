package discovery

import (
	"fmt"

	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

type DiscoveryResult struct {
	Source  string
	Results DiscoverAndCreateIssueResults
}

func DiscoverAll(baseURL string, opts http_utils.HistoryCreationOptions) ([]DiscoveryResult, error) {
	var allResults []DiscoveryResult
	var errors []error

	discoveryFunctions := map[string]func(string, http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error){
		"openapi":        DiscoverOpenapiDefinitions,
		"graphql":        DiscoverGraphQLEndpoints,
		"kubernetes":     DiscoverKubernetesEndpoints,
		"wordpress":      DiscoverWordPressEndpoints,
		"config_files":   DiscoverSensitiveConfigFiles,
		"actuator":       DiscoverActuatorEndpoints,
		"docker":         DiscoverDockerAPIEndpoints,
		"metrics":        DiscoveMetricsEndpoints,
		"vcs":            DiscoverVersionControlFiles,
		"cicd":           DiscoverCICDBuildFiles,
		"admin":          DiscoverAdminInterfaces,
		"db_management":  DiscoverDBManagementInterfaces,
		"cloud_metadata": DiscoverCloudMetadata,
		"sso":            DiscoverSSOEndpoints,
		"oauth":          DiscoverOAuthEndpoints,
		"phpinfo":        DiscoverPHPInfo,
		"payment_test":   DiscoverPaymentTestEndpoints,
		"socketio":       DiscoverSocketIO,
		"server_info":    DiscoverServerInfo,
		"logs":           DiscoverLogFiles,
		"jmx":            DiscoverHTTPJMXEndpoints,
		"axis2":          DiscoverAxis2Endpoints,
		"grpc":           DiscoverGRPCEndpoints,
		"wsdl":           DiscoverWSDLDefinitions,
		"trace_axd":      DiscoverAspNetTrace,
	}

	for source, discoverFunc := range discoveryFunctions {
		log.Info().Str("check", source).Msg("Discovering hidden paths")
		results, err := discoverFunc(baseURL, opts)
		if err != nil {
			errors = append(errors, fmt.Errorf("%s discovery failed: %w", source, err))
		} else {
			allResults = append(allResults, DiscoveryResult{
				Source:  source,
				Results: results,
			})
		}
	}

	if len(errors) > 0 {
		return allResults, fmt.Errorf("some discoveries failed: %v", errors)
	}

	return allResults, nil
}
