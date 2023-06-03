package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

// Interesting: https://github.com/filedescriptor/untrusted-types

type PageDOMAudit struct {
	URL      string
	Page     *rod.Page
	MaxDepth int
	// Pierce (optional) Whether or not iframes and shadow roots should be traversed when returning the subtree
	// (default is false). Reports listeners for all contexts if pierce is enabled.
	Pierce bool
}

func (a *PageDOMAudit) Run() {
	logger := log.With().Str("url", a.URL).Logger()

	document, err := proto.DOMGetDocument{
		Depth: &a.MaxDepth,
	}.Call(a.Page)
	if err != nil {
		logger.Warn().Msg("Could not get DOM document during DOM audit")
	}
	logger.Info().Interface("document", document).Msg("Root document node gathered")

	a.InspectDOMNodes(document.Root.Children)

	// https://chromedevtools.github.io/devtools-protocol/tot/DOMDebugger/#method-getEventListeners
	// allHtml, err := a.Page.Element("html")
	// if err != nil {
	// 	logger.Warn().Msg("Could not get page HTML during DOM audit")
	// }

	// eventListeners, err := proto.DOMDebuggerGetEventListeners{
	// 	ObjectID: allHtml.Object.ObjectID,
	// 	Depth:    a.MaxDepth,
	// 	Pierce:   a.Pierce,
	// }.Call(a.Page)
	// if err != nil {
	// 	logger.Warn().Msg("Could not get event listeners")
	// }

	// for _, listener := range eventListeners.Listeners {
	// 	log.Info().Interface("listener", listener).Int("node", int(listener.BackendNodeID)).Msg("DOM Listeners")
	// 	log.Info().Interface("handler", listener.Handler).Interface("original", listener.OriginalHandler).Msg("handler")
	// }
	// stackTraces := proto.DOMGetNodeStackTraces{
	// 	NodeID: allHtml.MustNodeID(),
	// }
	// log.Info().Interface("trace", stackTraces).Msg("Stack trace")

}

func (a *PageDOMAudit) InspectDOMNodes(nodes []*proto.DOMNode) {
	for _, node := range nodes {
		a.InspectDOMNode(node)
	}
	// a.Page.
}

func (a *PageDOMAudit) InspectDOMNode(node *proto.DOMNode) {
	// Interesting: https://chromedevtools.github.io/devtools-protocol/tot/DOM/#method-getNodeStackTraces
	log.Info().Interface("node", node).Msg("Node detail info")
	if *node.ChildNodeCount > 0 {
		a.InspectDOMNodes(node.Children)
	}
}

func (a *PageDOMAudit) EvaluateCookiesUsageInDOM() {
	// Should get all cookies with httpOnly: false and check if its value is used somewhere
}
