package integrations

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	pb "github.com/pyneda/nuclei-api/pkg/service"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func processNucleiResult(result *pb.ScanResult, workspaceID uint) {
	var info pb.ScanResultInfo
	if result == nil {
		log.Error().Uint("workspace", workspaceID).Str("id", result.TemplateId).Interface("result", result).Msg("Received nuclei scan result without enough information")
		return
	}

	if result.Info != nil {
		info = *result.Info
	} else {
		info.Name = convertToTitle(result.TemplateId)
		info.Description = "N/A"
		info.Severity = "Unknown"
		info.References = []string{}
		info.Remediation = "N/A"
	}

	var sb strings.Builder
	if result.MatcherStatus && result.MatcherName != "" {
		sb.WriteString("Matched: " + result.MatcherName)
		if result.Matched != "" {
			sb.WriteString("\nMatched value: " + result.Matched)
		}
		sb.WriteString("\n\n")
	}

	if len(result.ExtractedResults) > 0 {
		sb.WriteString("Extracted results: ")
		for _, extractedResult := range result.ExtractedResults {
			sb.WriteString(extractedResult)
		}
	}

	if result.Interaction != nil {
		sb.WriteString("An out of band " + result.Interaction.Protocol + " interaction has been detected.\n\n")
		sb.WriteString("The interaction originated from " + result.Interaction.RemoteAddress + " and was performed at " + result.Interaction.Timestamp + ".\n\nFind below the request data:\n")
		sb.WriteString(string(result.Interaction.RawRequest) + "\n\n")
		sb.WriteString("The server responded with the following data:\n")
		sb.WriteString(string(result.Interaction.RawResponse) + "\n")
	}

	sb.WriteString("\n\nNOTE: This issue has been generated via the Nuclei integration using a template with ID: " + result.TemplateId)

	issue := db.Issue{
		Code:        result.TemplateId,
		Title:       info.Name,
		Description: info.Description,
		Remediation: info.Remediation,
		URL:         result.Matched,
		Details:     sb.String(),
		Request:     []byte(result.Request),
		Response:    []byte(result.Response),
		References:  info.References,
		CURLCommand: result.CurlCommand,
		Severity:    db.NewSeverity(lib.CapitalizeFirstLetter(info.Severity)),
		WorkspaceID: &workspaceID,
	}

	new, err := db.Connection().CreateIssue(issue)
	if err != nil {
		log.Error().Uint("workspace", workspaceID).Err(err).Interface("issue", issue).Msg("Could not create nuclei issue")
		return
	}
	log.Info().Uint("workspace", workspaceID).Interface("issue", new).Msg("Created nuclei issue")

}

func NucleiScan(targets []string, workspaceID uint) error {
	address := fmt.Sprintf("%v:%v", viper.Get("integrations.nuclei.host"), viper.Get("integrations.nuclei.port"))
	scanRequest := &pb.ScanRequest{
		Targets:           targets,
		AutomaticScan:     viper.GetBool("integrations.nuclei.automatic_scan"),
		Tags:              viper.GetStringSlice("integrations.nuclei.tags"),
		ExcludeTags:       viper.GetStringSlice("integrations.nuclei.exclude_tags"),
		Workflows:         viper.GetStringSlice("integrations.nuclei.workflows"),
		WorkflowUrls:      viper.GetStringSlice("integrations.nuclei.exclude_workflows"),
		Templates:         viper.GetStringSlice("integrations.nuclei.templates"),
		ExcludedTemplates: viper.GetStringSlice("integrations.nuclei.excluded_templates"),
		ExcludeMatchers:   viper.GetStringSlice("integrations.nuclei.exclude_matchers"),
		CustomHeaders:     viper.GetStringSlice("integrations.nuclei.custom_headers"),
		Severities:        viper.GetStringSlice("integrations.nuclei.severities"),
		ExcludeSeverities: viper.GetStringSlice("integrations.nuclei.exclude_severities"),
		Authors:           viper.GetStringSlice("integrations.nuclei.authors"),
		Protocols:         viper.GetStringSlice("integrations.nuclei.protocols"),
		ExcludeProtocols:  viper.GetStringSlice("integrations.nuclei.exclude_protocols"),
		IncludeTags:       viper.GetStringSlice("integrations.nuclei.include_tags"),
		IncludeTemplates:  viper.GetStringSlice("integrations.nuclei.include_ids"),
		IncludeIds:        viper.GetStringSlice("integrations.nuclei.include_ids"),
		ExcludeIds:        viper.GetStringSlice("integrations.nuclei.exclude_ids"),
		Headless:          viper.GetBool("integrations.nuclei.headless"),
		NewTemplates:      viper.GetBool("integrations.nuclei.new_templates"),
	}

	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Error().Uint("workspace", workspaceID).Err(err).Msg("Could not connect to nuclei-api")
		return err
	}
	defer conn.Close()
	c := pb.NewNucleiApiClient(conn)

	// Contact the server and print out its response.
	timeout := time.Duration(viper.GetInt("integrations.nuclei.scan_timeout"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*timeout)
	defer cancel()
	log.Info().Uint("workspace", workspaceID).Interface("configuration", scanRequest).Msg("Starting nuclei scan through nuclei-api")
	stream, err := c.Scan(ctx, scanRequest)
	if err != nil {
		log.Error().Err(err).Msg("Error starting a nuclei scan through nuclei-api")
		return err
	}
	for {
		result, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Error().Uint("workspace", workspaceID).Err(err).Msg("Error while performing a nuclei scan through nuclei-api")
			return err
		}
		log.Info().Uint("workspace", workspaceID).Str("id", result.TemplateId).Msg("Received nuclei scan result")
		processNucleiResult(result, workspaceID)
	}
	return nil
}
