package integrations

import (
	"context"
	"fmt"
	pb "github.com/pyneda/nuclei-api/pkg/service"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"io"
	"strings"
	"time"
)

func processNucleiResult(result *pb.ScanResult) {
	if result == nil || result.Info == nil {
		log.Error().Str("id", result.TemplateId).Interface("result", result).Msg("Received nuclei scan result without enough information")
		return
	}

	var sb strings.Builder
	if result.MatcherStatus && result.MatcherName != "" {
		sb.WriteString("Matched: " + result.MatcherName)
		if result.Matched != "" {
			sb.WriteString("Matched value: " + result.Matched)
		}
	}

	if len(result.ExtractedResults) > 0 {
		sb.WriteString("Extracted results: ")
		for _, extractedResult := range result.ExtractedResults {
			sb.WriteString(extractedResult)
		}
	}

	sb.WriteString("\n\nNOTE: This issue has been generated via the Nuclei integration using a template with ID: " + result.TemplateId)

	issue := db.Issue{
		Code:        result.TemplateId,
		Title:       result.Info.Name,
		Description: result.Info.Description,
		Remediation: result.Info.Remediation,
		URL:         result.Matched,
		Details:     sb.String(),
		Request:     []byte(result.Request),
		Response:    []byte(result.Response),
		References:  result.Info.References,
		CURLCommand: result.CurlCommand,
		Severity:    lib.CapitalizeFirstLetter(result.Info.Severity),
	}

	new, err := db.Connection.CreateIssue(issue)
	if err != nil {
		log.Error().Err(err).Interface("issue", issue).Msg("Could not create nuclei issue")
		return
	}
	log.Info().Interface("issue", new).Msg("Created nuclei issue")

}

func NucleiScan(targets []string) error {
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
		log.Error().Err(err).Msg("Could not connect to nuclei-api")
		return err
	}
	defer conn.Close()
	c := pb.NewNucleiApiClient(conn)

	// Contact the server and print out its response.
	timeout := time.Duration(viper.GetInt("integrations.nuclei.scan_timeout"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*timeout)
	defer cancel()
	log.Info().Interface("configuration", scanRequest).Msg("Starting nuclei scan through nuclei-api")
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
			log.Error().Err(err).Msg("Error while performing a nuclei scan through nuclei-api")
			return err
		}
		log.Info().Str("id", result.TemplateId).Msg("Received nuclei scan result")
		processNucleiResult(result)
	}
	return nil
}
