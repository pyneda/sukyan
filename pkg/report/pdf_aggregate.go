package report

import (
	"net/url"
	"sort"
)

type HostRollup struct {
	Host     string
	URLCount int
}

// pdfFinding is the PDF's presentation view over a shared GroupedIssues. It is a
// separate type rather than extra fields on GroupedIssues, which is serialised
// into the HTML report's embedded payload and must not change.
type pdfFinding struct {
	Group          *GroupedIssues
	UniqueURLs     int
	Hosts          []HostRollup
	Instances      []*ReportIssue
	TotalInstances int
}

func (f pdfFinding) Capped() bool { return len(f.Instances) < f.TotalInstances }

func (f pdfFinding) OmittedInstances() int { return f.TotalInstances - len(f.Instances) }

// OmittedURLs is the number of distinct URLs that appear only in instances the
// cap removed, so the disclosure line can state what the reader is not seeing.
func (f pdfFinding) OmittedURLs() int {
	if !f.Capped() {
		return 0
	}

	shown := make(map[string]bool, len(f.Instances))
	for _, issue := range f.Instances {
		shown[issue.URL] = true
	}
	return f.UniqueURLs - len(shown)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

// buildPDFFindings derives per-finding rollups and applies the instance cap.
// Rollups count every instance even when the rendered list is capped, so the
// disclosure line reports true totals.
func buildPDFFindings(groups []*GroupedIssues, maxInstances int) []pdfFinding {
	findings := make([]pdfFinding, 0, len(groups))

	for _, g := range groups {
		urlsByHost := make(map[string]map[string]bool)
		uniqueURLs := make(map[string]bool)

		for _, issue := range g.Issues {
			h := hostOf(issue.URL)
			if urlsByHost[h] == nil {
				urlsByHost[h] = make(map[string]bool)
			}
			urlsByHost[h][issue.URL] = true
			uniqueURLs[issue.URL] = true
		}

		hosts := make([]HostRollup, 0, len(urlsByHost))
		for h, urls := range urlsByHost {
			hosts = append(hosts, HostRollup{Host: h, URLCount: len(urls)})
		}
		sort.Slice(hosts, func(i, j int) bool {
			if hosts[i].URLCount != hosts[j].URLCount {
				return hosts[i].URLCount > hosts[j].URLCount
			}
			return hosts[i].Host < hosts[j].Host
		})

		instances := g.Issues
		if maxInstances > 0 && len(instances) > maxInstances {
			instances = instances[:maxInstances]
		}

		findings = append(findings, pdfFinding{
			Group:          g,
			UniqueURLs:     len(uniqueURLs),
			Hosts:          hosts,
			Instances:      instances,
			TotalInstances: len(g.Issues),
		})
	}

	return findings
}
