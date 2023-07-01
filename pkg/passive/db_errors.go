package passive

import "regexp"

// Patterns taken from: https://github.com/stamparm/DSSS/blob/master/dsss.py

var DBMS_ERRORS = map[string][]*regexp.Regexp{
	"MySQL": compilePatterns(
		`SQL syntax.*MySQL`,
		`Warning.*mysql_.*`,
		`valid MySQL result`,
		`MySqlClient\.`),
	"PostgreSQL": compilePatterns(
		`PostgreSQL.*ERROR`,
		`Warning.*\Wpg_.*`,
		`valid PostgreSQL result`,
		`Npgsql\.`),
	"Microsoft SQL Server": compilePatterns(
		`Driver.* SQL[\-\_\ ]*Server`,
		`OLE DB.* SQL Server`,
		`(\W|\A)SQL Server.*Driver`,
		`Warning.*mssql_.*`,
		`(\W|\A)SQL Server.*[0-9a-fA-F]{8}`,
		`(?s)Exception.*\WSystem\.Data\.SqlClient\.`,
		`(?s)Exception.*\WRoadhouse\.Cms\.`),
	"Microsoft Access": compilePatterns(
		`Microsoft Access Driver`,
		`JET Database Engine`,
		`Access Database Engine`),
	"Oracle": compilePatterns(
		`\bORA-[0-9][0-9][0-9][0-9]`,
		`Oracle error`,
		`Oracle.*Driver`,
		`Warning.*\Woci_.*`,
		`Warning.*\Wora_.*`),
	"IBM DB2": compilePatterns(
		`CLI Driver.*DB2`,
		`DB2 SQL error`,
		`\bdb2_\w+\(`),
	"SQLite": compilePatterns(
		`SQLite/JDBCDriver`,
		`SQLite.Exception`,
		`System.Data.SQLite.SQLiteException`,
		`Warning.*sqlite_.*`,
		`Warning.*SQLite3::`,
		`\[SQLITE_ERROR\]`),
	"Sybase": compilePatterns(
		`(?i)Warning.*sybase.*`,
		`Sybase message`,
		`Sybase.*Server message.*`),
}

type DatabaseErrorMatch struct {
	DatabaseName string
	MatchStr     string
}

func SearchDatabaseErrors(text string) *DatabaseErrorMatch {
	for db, patterns := range DBMS_ERRORS {
		for _, pattern := range patterns {
			matchStr := pattern.FindString(text)
			if matchStr != "" {
				return &DatabaseErrorMatch{DatabaseName: db, MatchStr: matchStr}
			}
		}
	}
	return nil
}
