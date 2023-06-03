package passive

import "regexp"

func GetDatabaseErrorRegxes() map[string][]regexp.Regexp {
	// Taken from: https://github.com/stamparm/DSSS/blob/master/dsss.py
	return map[string][]regexp.Regexp{
		"MySQL":      GetMysqlErrorPatterns(),
		"PostgreSQL": GetPsqlErrorPatterns(),
		// "Microsoft SQL Server": GetMssqlErrorPatterns(),
		"SQLite": GetSqliteErrorPatterns(),
		// "Oracle":               GetOracleErrorPatterns(),
		// "IBM DB2":              GetIbmDb2ErrorPatterns(),
		// "Microsoft Access":     GetAccessErrorPatterns(),
		// "Sybase":               GetSybaseErrorPatterns(),
	}
}

var (
	// MySQL
	MySQLSyntaxRegex      = regexp.MustCompile(`SQL syntax.*MySQL`)
	MySQLWarningRegex     = regexp.MustCompile(`Warning.*mysql_.*`)
	MySQLValidResultRegex = regexp.MustCompile(`valid MySQL result`)
	MySQLClientRegex      = regexp.MustCompile(`MySqlClient\.`)
	// PostgreSQL
	PostgreSQLErrorRegex       = regexp.MustCompile(`PostgreSQL.*ERRO`)
	PostgreSQLWarningRegex     = regexp.MustCompile(`Warning.*\Wpg_.*`)
	PostgreSQLValidResultRegex = regexp.MustCompile(`valid PostgreSQL result`)
	PostgreSQLNpgsqlRegex      = regexp.MustCompile(`Npgsql\.`)
	// SQlite
	SQLiteJDBCDriveRegex       = regexp.MustCompile(`SQLite/JDBCDrive`)
	SQLiteExceptionRegex       = regexp.MustCompile(`SQLite.Exception`)
	SQLiteSystemExceptionRegex = regexp.MustCompile(`System.Data.SQLite.SQLiteException`)
	SQLiteWarningRegex         = regexp.MustCompile(`Warning.*sqlite_.*`)
	SQLiteErrorRegex           = regexp.MustCompile(`\[SQLITE_ERROR\]`)
)

func GetSqliteErrorPatterns() []regexp.Regexp {
	return []regexp.Regexp{
		*SQLiteJDBCDriveRegex, *SQLiteErrorRegex, *SQLiteExceptionRegex, *SQLiteSystemExceptionRegex, *SQLiteWarningRegex,
	}
}

func GetPsqlErrorPatterns() []regexp.Regexp {
	return []regexp.Regexp{
		*PostgreSQLErrorRegex, *PostgreSQLWarningRegex, *PostgreSQLValidResultRegex, *PostgreSQLNpgsqlRegex,
	}
}

func GetMysqlErrorPatterns() []regexp.Regexp {
	return []regexp.Regexp{
		*MySQLSyntaxRegex, *MySQLClientRegex, *MySQLValidResultRegex, *MySQLClientRegex,
	}
}

// func GetMssqlErrorPatterns() []string {
// 	return []string{
// 		`Driver.* SQL[\-\_\ ]*Serve`, `OLE DB.* SQL Serve`, `(\W|\A)SQL Server.*Drive`, `Warning.*mssql_.*`, `(\W|\A)SQL Server.*[0-9a-fA-F]{8}`, `(?s)Exception.*\WSystem\.Data\.SqlClient\.`, `(?s)Exception.*\WRoadhouse\.Cms\.`,
// 	}
// }

// func GetOracleErrorPatterns() []string {
// 	return []string{
// 		`\bORA-[0-9][0-9][0-9][0-9]`, `Oracle erro`, `Oracle.*Drive`, `Warning.*\Woci_.*`, `Warning.*\Wora_.*`,
// 	}
// }

// func GetAccessErrorPatterns() []string {
// 	return []string{
// 		`Microsoft Access Drive`, `JET Database Engine`, `Access Database Engine`,
// 	}
// }

// func GetIbmDb2ErrorPatterns() []string {
// 	return []string{
// 		`CLI Driver.*DB2`, `DB2 SQL erro`, `\bdb2_\w+\(`,
// 	}
// }

// func GetSybaseErrorPatterns() []string {
// 	return []string{
// 		`(?i)Warning.*sybase.*`, `Sybase message`, `Sybase.*Server message.*`,
// 	}
// }
