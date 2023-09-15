package integrations

import (
	"github.com/projectdiscovery/cdncheck"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"net"
)

// Utility function to build the details string
func buildDetails(ip net.IP, val string, extraInfo string) string {
	return ip.String() + " (" + val + ")\n\n" + extraInfo
}

func checkCDN(client *cdncheck.Client, ip net.IP, urlStr string, workspaceID uint) (db.Issue, error) {
	matched, val, err := client.CheckCDN(ip)
	if err != nil {
		log.Error().Err(err).Str("check", "cdncheck").Uint("workspace", workspaceID).Msg("Error during CDN check")
		return db.Issue{}, err
	}

	if matched {
		issue := db.GetIssueTemplateByCode(db.CdnDetectedCode)
		issue.URL = urlStr
		extraInfo := "This issue has been detected by using cdncheck (https://github.com/projectdiscovery/cdncheck)"
		issue.Details = buildDetails(ip, val, extraInfo)
		createdIssue, err := db.Connection.CreateIssue(*issue)
		if err != nil {
			return db.Issue{}, err
		}
		log.Info().Str("check", "cdncheck").Uint("workspace", workspaceID).Msgf("%v is a %v", ip, val)
		return createdIssue, nil
	}
	return db.Issue{}, nil
}

func checkCloud(client *cdncheck.Client, ip net.IP, urlStr string, workspaceID uint) (db.Issue, error) {
	matched, val, err := client.CheckCloud(ip)
	if err != nil {
		log.Error().Err(err).Str("check", "cdncheck").Uint("workspace", workspaceID).Msg("Error during Cloud check")
		return db.Issue{}, err
	}

	if matched {
		issue := db.GetIssueTemplateByCode(db.CloudDetectedCode)
		issue.URL = urlStr
		extraInfo := "This issue has been detected by using cdncheck (https://github.com/projectdiscovery/cdncheck)"
		issue.Details = buildDetails(ip, val, extraInfo)
		createdIssue, err := db.Connection.CreateIssue(*issue)
		if err != nil {
			return db.Issue{}, err
		}
		log.Info().Str("check", "cdncheck").Uint("workspace", workspaceID).Msgf("%v is a %v", ip, val)
		return createdIssue, nil
	}
	return db.Issue{}, nil
}

func checkWAF(client *cdncheck.Client, ip net.IP, urlStr string, workspaceID uint) (db.Issue, error) {
	matched, val, err := client.CheckWAF(ip)
	if err != nil {
		log.Error().Err(err).Str("check", "cdncheck").Uint("workspace", workspaceID).Msg("Error during WAF check")
		return db.Issue{}, err
	}

	if matched {
		issue := db.GetIssueTemplateByCode(db.WafDetectedCode)
		issue.URL = urlStr
		extraInfo := "This issue has been detected by using cdncheck (https://github.com/projectdiscovery/cdncheck)"
		issue.Details = buildDetails(ip, val, extraInfo)
		createdIssue, err := db.Connection.CreateIssue(*issue)
		if err != nil {
			return db.Issue{}, err
		}
		log.Info().Str("check", "cdncheck").Uint("workspace", workspaceID).Msgf("%v is a %v", ip, val)
		return createdIssue, nil
	}
	return db.Issue{}, nil
}

func CDNCheck(urlStr string, workspaceID uint) ([]db.Issue, error) {
	var issues []db.Issue

	ips, err := lib.GetIPFromURL(urlStr)
	if err != nil {
		log.Error().Err(err).Str("check", "cdncheck").Uint("workspace", workspaceID).Msg("Error resolving URL to IP")
		return issues, err
	}

	client := cdncheck.New()
	for _, ip := range ips {
		log.Info().Str("check", "cdncheck").Uint("workspace", workspaceID).Msgf("Performing checks for IP: %v", ip)

		issue, err := checkCDN(client, ip, urlStr, workspaceID)
		if err == nil && !issue.IsEmpty() {
			issues = append(issues, issue)
		}

		issue, err = checkCloud(client, ip, urlStr, workspaceID)
		if err == nil && !issue.IsEmpty() {
			issues = append(issues, issue)
		}

		issue, err = checkWAF(client, ip, urlStr, workspaceID)
		if err == nil && !issue.IsEmpty() {
			issues = append(issues, issue)
		}

	}

	return issues, nil
}
