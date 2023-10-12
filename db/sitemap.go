package db

import (
	"github.com/pyneda/sukyan/lib"
	"net/url"
	"path/filepath"
	"strings"
)

type SitemapNode struct {
	ID       uint            `json:"id"`
	OtherIDs []uint          `json:"other_ids,omitempty"`
	Depth    int             `json:"depth"`
	URL      string          `json:"url"`
	Path     string          `json:"path"`
	Type     SitemapNodeType `json:"type"`
	Children []*SitemapNode  `json:"children"`
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
)

func (d *DatabaseConnection) getSitemapData(filter SitemapFilter) ([]History, error) {
	query := d.db.Model(&History{})
	if filter.WorkspaceID != 0 {
		query = query.Where("workspace_id = ?", filter.WorkspaceID)
	}
	if filter.TaskID != 0 {
		query = query.Where("task_id = ?", filter.TaskID)
	}
	var histories []History
	err := query.Find(&histories).Error
	if err != nil {
		return nil, err
	}
	return histories, nil
}

func (d *DatabaseConnection) ConstructSitemap(filter SitemapFilter) ([]*SitemapNode, error) {
	histories, err := d.getSitemapData(filter)
	if err != nil {
		return nil, err
	}

	nodes := make(map[string]*SitemapNode)
	const maxUint = ^uint(0)
	nextNegativeID := maxUint // for URLs without a history or missing paths
	for _, history := range histories {
		baseURL, err := lib.GetBaseURL(history.URL)
		if err != nil {
			return nil, err
		}

		if _, exists := nodes[baseURL]; !exists {
			node := &SitemapNode{
				ID:       history.ID, // set the ID for the baseURL the first time it's encountered
				Depth:    0,
				URL:      baseURL,
				Type:     SitemapNodeTypeRoot,
				Path:     "",
				Children: []*SitemapNode{},
			}
			nodes[baseURL] = node
		} else {
			nodes[baseURL].OtherIDs = append(nodes[baseURL].OtherIDs, history.ID)
		}

		currentNode := nodes[baseURL]
		u, err := url.Parse(history.URL)
		if err != nil {
			return nil, err
		}

		parts := strings.Split(u.Path, "/")
		for i, part := range parts[1:] {
			if part == "" {
				continue
			}

			childNode := findChildByPath(currentNode, part)
			if childNode == nil {
				childUrl := baseURL + strings.Join(parts[:i+2], "/")
				childNode = &SitemapNode{
					ID:       history.ID,
					Depth:    currentNode.Depth + 1,
					URL:      childUrl,
					Path:     part,
					Type:     determineType(childUrl),
					Children: []*SitemapNode{},
				}
				currentNode.Children = append(currentNode.Children, childNode)
			} else {
				childNode.OtherIDs = append(childNode.OtherIDs, history.ID)
			}
			currentNode = childNode
		}

		if u.RawQuery != "" {
			queryChild := &SitemapNode{
				ID:       history.ID, // nextNegativeID,
				Depth:    currentNode.Depth + 1,
				URL:      history.URL,
				Type:     SitemapNodeTypeQuery,
				Path:     u.RawQuery,
				Children: []*SitemapNode{},
			}
			currentNode.Children = append(currentNode.Children, queryChild)
			nextNegativeID--
		}
	}

	var results []*SitemapNode
	for _, node := range nodes {
		results = append(results, node)
	}

	return results, nil
}

func findChildByPath(node *SitemapNode, path string) *SitemapNode {
	for _, child := range node.Children {
		if child.Path == path {
			return child
		}
	}
	return nil
}

// determineType returns the SitemapNodeType based on the URL and its properties.
func determineType(urlStr string) SitemapNodeType {
	// Parse the URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return SitemapNodeTypeFile // Default to file if there's an error
	}

	// Check for root
	if u.Path == "/" || u.Path == "" {
		return SitemapNodeTypeRoot
	}

	// Check for query
	if u.RawQuery != "" {
		return SitemapNodeTypeQuery
	}

	// Check for directory (treat as directory if not recognized as a specific file type)
	ext := filepath.Ext(u.Path)
	if ext == "" || (ext != "" && determineFileType(ext) == SitemapNodeTypeFile) {
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
	return SitemapNodeTypeFile
}
