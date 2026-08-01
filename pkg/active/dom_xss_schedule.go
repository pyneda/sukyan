package active

import (
	"time"

	"github.com/pyneda/sukyan/pkg/web"
)

type domXSSSourceKind int

const (
	domXSSSourceKindURL domXSSSourceKind = iota
	domXSSSourceKindStorage
	domXSSSourceKindOther
	domXSSSourceKindPostMessage
)

// domXSSSourcePlan pairs a source with the tester that knows how to drive it.
type domXSSSourcePlan struct {
	Source web.DOMXSSSource
	Kind   domXSSSourceKind
}

// planDOMXSSSources lists every source the audit will try, in priority order.
func planDOMXSSSources(includeStorage, includePostMessage bool) []domXSSSourcePlan {
	var plan []domXSSSourcePlan

	for _, source := range web.GetURLBasedSources() {
		plan = append(plan, domXSSSourcePlan{Source: source, Kind: domXSSSourceKindURL})
	}

	if includeStorage {
		for _, source := range web.GetStorageSources() {
			plan = append(plan, domXSSSourcePlan{Source: source, Kind: domXSSSourceKindStorage})
		}
	}

	for _, source := range web.DOMXSSSources() {
		if source.Type == web.SourceTypeDocument || source.Type == web.SourceTypeWindow {
			plan = append(plan, domXSSSourcePlan{Source: source, Kind: domXSSSourceKindOther})
		}
	}

	if includePostMessage {
		for _, source := range web.GetMessageSources() {
			plan = append(plan, domXSSSourcePlan{Source: source, Kind: domXSSSourceKindPostMessage})
		}
	}

	return plan
}

// domXSSSourceBudget hands the next source an equal share of the time that is
// left, so one source cannot drain the whole audit and starve the rest. Time a
// source does not use rolls forward to the sources after it.
func domXSSSourceBudget(remaining time.Duration, sourcesLeft int) time.Duration {
	if remaining <= 0 {
		return 0
	}
	if sourcesLeft <= 1 {
		return remaining
	}
	return remaining / time.Duration(sourcesLeft)
}
