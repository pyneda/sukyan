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
		`sqlite3.OperationalError`,
		`sqlite3.ProgrammingError`,
		`\[SQLITE_ERROR\]`),
	"Sybase": compilePatterns(
		`(?i)Warning.*sybase.*`,
		`Sybase message`,
		`Sybase.*Server message.*`),
	"MongoDB": compilePatterns(
		`MongoError`,
		`failed to connect to server .* on first connect`,
		`E11000 duplicate key error collection`,
		`collection .* already exists`,
		`\bdeadlock\b.*\bdetected\b`,
		`unexpected token`,
		`invalid .* syntax`,
		`Failed to parse:.*'filter'.*`,
		`unknown operator:.*`,
		`No array filter found for identifier.*in path.*`,
		`Cannot use.*as a query operator`,
		`Cannot do exclusion on path.*in inclusion projection`,
		`Path.*intersects with a project inclusion`,
		`Unrecognized expression.*`,
		`is not a valid hex number`,
		`Failed to parse document from.*: *unexpected character.*after document key`,
	),
	"CouchDB": compilePatterns(
		`unauthorized to access or create database`,
		`no_db_file`,
		`document update conflict`,
		`invalid UTF-8 JSON`,
		`badmatch`,
	),
	"Cassandra": compilePatterns(
		`Cassandra.*InvalidQueryException`,
		`unterminated string`,
		`line .* no viable alternative at input`,
		`mismatched input .* expecting .*`,
	),
	"Redis": compilePatterns(
		`redis.*WRONGTYPE`,
		`redis.*syntax error`,
	),
	"Elasticsearch": compilePatterns(
		`SearchPhaseExecutionException`,
		`QueryParsingException`,
		`unexpected token`,
		`invalid .* syntax`,
	),
	"DynamoDB": compilePatterns(
		`ValidationException`,
		`com.amazonaws.services.dynamodbv2.model.AmazonDynamoDBException`,
		`ProvisionedThroughputExceededException`,
	),
	"HBase": compilePatterns(
		`org.apache.hadoop.hbase.DoNotRetryIOException`,
		`ERROR: org.apache.hadoop.hbase.MasterNotRunningException`,
		`org.apache.hadoop.hbase.regionserver.NoSuchColumnFamilyException`,
	),
	"Neo4j": compilePatterns(
		`Neo.ClientError.Statement.SyntaxError`,
		`org.neo4j.driver.v1.exceptions.ClientException`,
		`org.neo4j.driver.v1.exceptions.DatabaseException`,
	),
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
