package db

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pyneda/sukyan/lib"
)

// SitemapNode is one addressable point of the target's attack surface. URL is
// its identity — the path walk guarantees one node per distinct URL path.
//
// Aggregates are subtree-inclusive and deduplicated: a history record increments
// each node along its path exactly once, so nothing double-counts.
type SitemapNode struct {
	URL   string          `json:"url"`
	Path  string          `json:"path"`
	Depth int             `json:"depth"`
	Type  SitemapNodeType `json:"type"`

	Requests      int            `json:"requests"`
	Methods       []string       `json:"methods,omitempty"`
	StatusClasses map[string]int `json:"status_classes,omitempty"`
	// Derived from the URL on read; parameter names are not persisted anywhere.
	Params []string `json:"params,omitempty"`

	// Only meaningful on endpoints — a directory's is an arbitrary descendant.
	ExampleID uint `json:"example_id"`

	// Reserved: correlating findings and coverage needs a join through
	// issue_requests plus task correlation. Declared now so filling them in later
	// does not reshape the payload for every consumer.
	Issues  *SitemapIssueCounts `json:"issues"`
	Scanned *bool               `json:"scanned"`

	HasChildren bool           `json:"has_children"`
	Children    []*SitemapNode `json:"children"`
}

type SitemapIssueCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
}

type SitemapFilter struct {
	WorkspaceID uint `json:"workspace_id" validate:"omitempty,numeric"`
	TaskID      uint `json:"task_id" validate:"omitempty,numeric"`
}

type SitemapNodeType string

const (
	// Generic
	SitemapNodeTypeRoot      SitemapNodeType = "root"
	SitemapNodeTypeDirectory SitemapNodeType = "directory"
	SitemapNodeTypeFile      SitemapNodeType = "file"
	SitemapNodeTypeQuery     SitemapNodeType = "query"
	// Specific (file extensions)
	SitemapNodeTypePhp      SitemapNodeType = "php"
	SitemapNodeTypeAsp      SitemapNodeType = "asp"
	SitemapNodeTypeJsp      SitemapNodeType = "jsp"
	SitemapNodeTypeJs       SitemapNodeType = "js"
	SitemapNodeTypeCss      SitemapNodeType = "css"
	SitemapNodeTypeHtml     SitemapNodeType = "html"
	SitemapNodeTypeXml      SitemapNodeType = "xml"
	SitemapNodeTypeJson     SitemapNodeType = "json"
	SitemapNodeTypeYaml     SitemapNodeType = "yaml"
	SitemapNodeTypeSql      SitemapNodeType = "sql"
	SitemapNodeTypeImage    SitemapNodeType = "image"
	SitemapNodeTypeVideo    SitemapNodeType = "video"
	SitemapNodeTypeAudio    SitemapNodeType = "audio"
	SitemapNodeTypeMarkdown SitemapNodeType = "markdown"
	SitemapNodeTypeFont     SitemapNodeType = "font"
	SitemapNodeTypeText     SitemapNodeType = "text"
)

// Real endpoints have tens of parameters; a heavily crawled tracking pixel can
// produce thousands of one-off names that would bloat the payload for nothing.
const maxParamsPerNode = 200

func (d *DatabaseConnection) getSitemapData(filter SitemapFilter) ([]History, error) {
	query := d.db.Model(&History{}).Select("id, url, method, status_code")
	if filter.WorkspaceID != 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}

	sources := GetSitemapSources()
	query = query.Where("source IN ?", sources)

	// The tree is built first-seen, so an unordered scan would yield different
	// ExampleIDs between calls.
	query = query.Order("id asc")

	var histories []History
	err := query.Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return histories, nil
}

// Sets are collapsed to sorted slices in finalise so the payload is stable.
type sitemapBuilder struct {
	node    *SitemapNode
	methods map[string]struct{}
	params  map[string]struct{}
	kids    map[string]*sitemapBuilder
	order   []*sitemapBuilder
}

func newBuilder(url, path string, depth int, t SitemapNodeType, exampleID uint) *sitemapBuilder {
	return &sitemapBuilder{
		node: &SitemapNode{
			URL:           url,
			Path:          path,
			Depth:         depth,
			Type:          t,
			ExampleID:     exampleID,
			StatusClasses: map[string]int{},
			Children:      []*SitemapNode{},
		},
		methods: map[string]struct{}{},
		params:  map[string]struct{}{},
		kids:    map[string]*sitemapBuilder{},
	}
}

// Called exactly once per node per history record, which is what keeps the
// counts deduplicated.
func (b *sitemapBuilder) record(h History, params []string) {
	b.node.Requests++
	if h.Method != "" {
		b.methods[h.Method] = struct{}{}
	}
	b.node.StatusClasses[statusClass(h.StatusCode)]++
	for _, p := range params {
		if len(b.params) >= maxParamsPerNode {
			break
		}
		b.params[p] = struct{}{}
	}
}

func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	case code >= 100 && code < 200:
		return "1xx"
	default:
		// Transport failures have no status but are still surface worth seeing.
		return "none"
	}
}

// Query strings do not become nodes: every request to a path collapses into one
// endpoint carrying the union of parameter names seen, which stops a tracking
// endpoint from emitting hundreds of visually identical rows.
func (d *DatabaseConnection) ConstructSitemap(filter SitemapFilter) ([]*SitemapNode, error) {
	histories, err := d.getSitemapData(filter)
	if err != nil {
		return nil, err
	}

	roots := map[string]*sitemapBuilder{}
	var rootOrder []*sitemapBuilder

	for _, history := range histories {
		baseURL, err := lib.GetBaseURL(history.URL)
		if err != nil {
			// A single malformed URL should not fail the whole sitemap.
			continue
		}
		u, err := url.Parse(history.URL)
		if err != nil {
			continue
		}

		params := queryParamNames(u)

		root, exists := roots[baseURL]
		if !exists {
			root = newBuilder(baseURL, "", 0, SitemapNodeTypeRoot, history.ID)
			roots[baseURL] = root
			rootOrder = append(rootOrder, root)
		}
		root.record(history, params)

		current := root
		segments := strings.Split(u.Path, "/")
		for i, segment := range segments[1:] {
			if segment == "" {
				continue
			}
			child, ok := current.kids[segment]
			if !ok {
				childURL := baseURL + strings.Join(segments[:i+2], "/")
				child = newBuilder(childURL, segment, current.node.Depth+1, determineType(childURL), history.ID)
				current.kids[segment] = child
				current.order = append(current.order, child)
			}
			child.record(history, params)
			current = child
		}
	}

	results := make([]*SitemapNode, 0, len(rootOrder))
	for _, root := range rootOrder {
		results = append(results, root.finalise())
	}
	sortNodes(results)
	return results, nil
}

func (b *sitemapBuilder) finalise() *SitemapNode {
	b.node.Methods = sortedKeys(b.methods)
	b.node.Params = sortedKeys(b.params)

	children := make([]*SitemapNode, 0, len(b.order))
	for _, kid := range b.order {
		children = append(children, kid.finalise())
	}
	sortNodes(children)

	b.node.Children = children
	b.node.HasChildren = len(children) > 0
	return b.node
}

// Go randomises map iteration, so without an explicit sort the payload — and the
// rendered tree — changes on every request. Busiest first, path ascending to
// break ties so equal-weight siblings never swap places.
func sortNodes(nodes []*SitemapNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Requests != nodes[j].Requests {
			return nodes[i].Requests > nodes[j].Requests
		}
		if nodes[i].Path != nodes[j].Path {
			return nodes[i].Path < nodes[j].Path
		}
		return nodes[i].URL < nodes[j].URL
	})
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func queryParamNames(u *url.URL) []string {
	if u.RawQuery == "" {
		return nil
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil
	}
	var names []string
	for name := range values {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// determineType returns the SitemapNodeType based on the URL and its properties.
func determineType(urlStr string) SitemapNodeType {
	u, err := url.Parse(urlStr)
	if err != nil {
		return SitemapNodeTypeFile // Default to file if there's an error
	}

	if u.Path == "/" || u.Path == "" {
		return SitemapNodeTypeRoot
	}

	ext := filepath.Ext(u.Path)
	if ext == "" || determineFileType(ext) == SitemapNodeTypeFile {
		return SitemapNodeTypeDirectory
	}

	return determineFileType(ext)
}

func determineFileType(ext string) SitemapNodeType {
	switch strings.ToLower(ext) {
	case ".php":
		return SitemapNodeTypePhp
	case ".asp":
		return SitemapNodeTypeAsp
	case ".jsp":
		return SitemapNodeTypeJsp
	case ".js":
		return SitemapNodeTypeJs
	case ".css":
		return SitemapNodeTypeCss
	case ".html", ".htm":
		return SitemapNodeTypeHtml
	case ".xml":
		return SitemapNodeTypeXml
	case ".json":
		return SitemapNodeTypeJson
	case ".yaml", ".yml":
		return SitemapNodeTypeYaml
	case ".sql":
		return SitemapNodeTypeSql
	case ".txt":
		return SitemapNodeTypeText
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".ico", ".bmp", ".webp":
		return SitemapNodeTypeImage
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm":
		return SitemapNodeTypeVideo
	case ".mp3", ".wav", ".ogg", ".flac", ".aac", ".wma", ".m4a":
		return SitemapNodeTypeAudio
	case ".md", ".markdown", ".mdx", ".mdown":
		return SitemapNodeTypeMarkdown
	case ".ttf", ".otf", ".woff", ".woff2", ".eot":
		return SitemapNodeTypeFont
	default:
		return SitemapNodeTypeFile
	}
}
