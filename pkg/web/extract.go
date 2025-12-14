package web

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

// GetPageAnchors find anchors on the given page
func GetPageAnchors(p *rod.Page) (anchors []string, err error) {
	anchors = []string{}
	evalResult, err := p.Eval(GetLinks)
	if err != nil {
		return nil, err
	}
	for _, link := range evalResult.Value.Arr() {
		anchors = append(anchors, link.String())
	}
	// log.Info().Strs("anchors", anchors).Int("count", len(anchors)).Msg("Page anchors gathered")
	return anchors, nil
}

// GetPageResources gets all the page loaded resources, not used by now and probably can be removed as all requests are already hijacked
func GetPageResources(p *rod.Page) ([]byte, error) {
	response, err := proto.PageGetResourceTree{}.Call(p)
	if err != nil {
		return nil, err
	}
	fmt.Println(response.FrameTree)
	return nil, nil
}

// ExtractedScript represents JavaScript code extracted from an HTML document
type ExtractedScript struct {
	Code   string
	Source string // Description of where the script was found (inline, event handler, etc.)
}

// ExtractJavascriptFromHTML parses HTML and extracts all JavaScript code including:
// - Inline <script> tags
// - Event handler attributes (onclick, onload, etc.)
func ExtractJavascriptFromHTML(html []byte) []ExtractedScript {
	scripts := make([]ExtractedScript, 0)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		log.Debug().Err(err).Msg("Failed to parse HTML for JavaScript extraction")
		return scripts
	}

	// Extract inline scripts
	doc.Find("script").Each(func(i int, s *goquery.Selection) {
		// Skip external scripts (those with src attribute)
		if _, hasSrc := s.Attr("src"); hasSrc {
			return
		}

		// Skip non-JavaScript script types
		if scriptType, hasType := s.Attr("type"); hasType {
			scriptType = strings.ToLower(scriptType)
			if scriptType != "" &&
				scriptType != "text/javascript" &&
				scriptType != "application/javascript" &&
				scriptType != "module" {
				return
			}
		}

		code := strings.TrimSpace(s.Text())
		if code != "" {
			scripts = append(scripts, ExtractedScript{
				Code:   code,
				Source: fmt.Sprintf("Inline <script> tag #%d", i+1),
			})
		}
	})

	// Event handler attributes to look for
	eventHandlers := []string{
		"onclick", "ondblclick", "onmousedown", "onmouseup", "onmouseover",
		"onmousemove", "onmouseout", "onmouseenter", "onmouseleave",
		"onkeydown", "onkeypress", "onkeyup",
		"onload", "onunload", "onabort", "onerror",
		"onfocus", "onblur", "onchange", "onsubmit", "onreset", "onselect",
		"oninput", "oninvalid",
		"ondrag", "ondragend", "ondragenter", "ondragleave", "ondragover", "ondragstart", "ondrop",
		"onscroll", "onresize",
		"oncopy", "oncut", "onpaste",
		"oncontextmenu",
		"ontouchstart", "ontouchmove", "ontouchend", "ontouchcancel",
		"onanimationstart", "onanimationend", "onanimationiteration",
		"ontransitionend",
	}

	// Extract event handlers from all elements
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		for _, handler := range eventHandlers {
			if code, exists := s.Attr(handler); exists {
				code = strings.TrimSpace(code)
				if code != "" {
					tagName := goquery.NodeName(s)
					scripts = append(scripts, ExtractedScript{
						Code:   code,
						Source: fmt.Sprintf("Event handler '%s' on <%s> element", handler, tagName),
					})
				}
			}
		}
	})

	// Extract javascript: URLs from href attributes
	doc.Find("[href]").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			href = strings.TrimSpace(href)
			if strings.HasPrefix(strings.ToLower(href), "javascript:") {
				code := strings.TrimPrefix(href, "javascript:")
				code = strings.TrimPrefix(code, "JavaScript:")
				code = strings.TrimPrefix(code, "JAVASCRIPT:")
				if code != "" {
					tagName := goquery.NodeName(s)
					scripts = append(scripts, ExtractedScript{
						Code:   code,
						Source: fmt.Sprintf("javascript: URL in href on <%s> element", tagName),
					})
				}
			}
		}
	})

	return scripts
}
