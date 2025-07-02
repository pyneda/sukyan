package web

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
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
