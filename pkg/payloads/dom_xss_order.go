package payloads

import "github.com/pyneda/sukyan/pkg/web"

// OrderDOMXSSPayloadsBySinkFamily round-robins payloads across the execution
// context each one was written for. The canonical list is grouped by family, so
// a source that only gets a few attempts before its budget runs out would
// otherwise see thirteen HTML variations and never an eval payload.
func OrderDOMXSSPayloadsBySinkFamily(plds []DOMXSSPayload) []DOMXSSPayload {
	buckets := make(map[web.DOMXSSSinkType][]DOMXSSPayload)
	var families []web.DOMXSSSinkType

	for _, payload := range plds {
		family := web.SinkTypeHTMLExecution
		if len(payload.TargetSinks) > 0 {
			family = payload.TargetSinks[0]
		}
		if _, seen := buckets[family]; !seen {
			families = append(families, family)
		}
		buckets[family] = append(buckets[family], payload)
	}

	ordered := make([]DOMXSSPayload, 0, len(plds))
	for len(ordered) < len(plds) {
		for _, family := range families {
			if len(buckets[family]) == 0 {
				continue
			}
			ordered = append(ordered, buckets[family][0])
			buckets[family] = buckets[family][1:]
		}
	}
	return ordered
}
