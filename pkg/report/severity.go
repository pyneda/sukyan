package report

var severityRanks = map[string]int{
	"Critical": 5,
	"High":     4,
	"Medium":   3,
	"Low":      2,
	"Info":     1,
	"Unknown":  0,
}

func severityRank(s string) int {
	return severityRanks[s]
}
