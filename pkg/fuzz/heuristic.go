package fuzz

// HeuristicRecord (should be moved in fuzz or to its own package)
type HeuristicRecord struct {
	URL        string
	StatusCode int
	BodySize   int
	Matched    []string
}
