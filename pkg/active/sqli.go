package active

// https://github.com/payloadbox/sql-injection-payload-list

func getSQLiPrefixes() []string {
	return []string{
		" ",
		") ",
		"' ",
		"') ",
	}
}

func getSQLiSuffixes() []string {
	return []string{
		"",
		"-- -",
		"#",
		"%%16",
	}
}

func getSQLiBooleanTests() []string {
	return []string{
		"AND %d=%d",
		"OR NOT (%d>%d)",
	}
}

// GetSQLiTestData returns a map with data used for SQLi tests
func GetSQLiTestData() map[string][]string {
	return map[string][]string{
		"prefixes":      getSQLiPrefixes(),
		"suffixes":      getSQLiSuffixes(),
		"boolean_tests": getSQLiBooleanTests(),
	}
}
