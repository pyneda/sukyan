package passive

import (
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"unicode"
)

// Patterns taken from:
// https://github.com/stamparm/DSSS/blob/master/dsss.py
// https://github.com/sqlmapproject/sqlmap/blob/master/data/xml/errors.xml

var DBMS_ERRORS = map[string][]*regexp.Regexp{
	"MySQL": compilePatterns(
		`SQL syntax.*MySQL`,
		`Warning.*mysql_.*`,
		`valid MySQL result`,
		`MySQLSyntaxErrorException`,
		`valid MySQL result`,
		`Table '[^']+' doesn't exist`,
		`MySqlClient\.`,
		`SQL syntax.*?\WMySQL`,
		`Warning.*?\Wmysqli?_`,
		`check the manual that (corresponds to|fits) your MySQL server version`,
		`check the manual that (corresponds to|fits) your MariaDB server version`,
		`check the manual that (corresponds to|fits) your Drizzle server version`,
		`Unknown column '[^ ]+' in 'field list'`,
		`com\.mysql\.jdbc`,
		`Zend_Db_(Adapter|Statement)_Mysqli_Exception`,
		`Pdo[./_\\]Mysql`,
		`MySqlException`,
		`SQLSTATE\[\d+\]: Syntax error or access violation`,
		`MemSQL does not support this type of query`,
		`is not supported by MemSQL`,
		`You have an error in your SQL syntax`,
		`You have an error in your SQL syntax; check the manual that corresponds to your MySQL server version for the right syntax to use near`,
		`unsupported nested scalar subselect`),
	"PostgreSQL": compilePatterns(
		`Warning.*\Wpg_.*`,
		`valid PostgreSQL result`,
		`PG::([a-zA-Z]*)Error`,
		`Npgsql\.`,
		`PostgreSQL.*?ERROR`,
		`PG::SyntaxError:`,
		`org\.postgresql\.util\.PSQLException`,
		`ERROR:\s\ssyntax error at or near`,
		`ERROR: parser: parse error at or near`,
		`PostgreSQL query failed`,
		`org\.postgresql\.jdbc`,
		`Pdo[./_\\]Pgsql`,
		`PSQLException`),
	"Microsoft SQL Server": compilePatterns(
		`Driver.* SQL[\-\_\ ]*Server`,
		`OLE DB.* SQL Server`,
		`(\W|\A)SQL Server.*Driver`,
		`Warning.*mssql_.*`,
		`Procedure or function .* expects parameter`,
		`Syntax error .* in query expression`,
		`SQL Server.*[0-9a-fA-F]{8}`,
		`(\W|\A)SQL Server.*[0-9a-fA-F]{8}`,
		`(?s)Exception.*\WSystem\.Data\.SqlClient\.`,
		`(?s)Exception.*\WRoadhouse\.Cms\.`,
		`OLE DB.*? SQL Server`,
		`\bSQL Server[^&lt;&quot;]+Driver`,
		`Warning.*?\W(mssql|sqlsrv)_`,
		`\bSQL Server[^&lt;&quot;]+[0-9a-fA-F]{8}`,
		`System\.Data\.SqlClient\.(SqlException|SqlConnection\.OnError)`,
		`Microsoft SQL Native Client error '[0-9a-fA-F]{8}`,
		`\[SQL Server\]`,
		`ODBC SQL Server Driver`,
		`ODBC Driver \d+ for SQL Server`,
		`SQLServer JDBC Driver`,
		`com\.jnetdirect\.jsql`,
		`macromedia\.jdbc\.sqlserver`,
		`Zend_Db_(Adapter|Statement)_Sqlsrv_Exception`,
		`com\.microsoft\.sqlserver\.jdbc`,
		`Pdo[./_\\](Mssql|SqlSrv)`,
		`SQL(Srv|Server)Exception`,
		`Unclosed quotation mark after the character string`),
	"Microsoft Access": compilePatterns(
		`Microsoft Access Driver`,
		`JET Database Engine`,
		`Access Database Engine`,
		`Microsoft Access (\d+ )?Driver`,
		`ODBC Microsoft Access`,
		`Syntax error \(missing operator\) in query expression`),
	"Oracle": compilePatterns(
		`Oracle error`,
		`Oracle.*Driver`,
		`Warning.*\Woci_.*`,
		`Warning.*\Wora_.*`,
		`\bORA-\d{5}`,
		`quoted string not properly terminated`,
		`SQL command not properly ended`,
		`macromedia\.jdbc\.oracle`,
		`oracle\.jdbc`,
		`Zend_Db_(Adapter|Statement)_Oracle_Exception`,
		`Pdo[./_\\](Oracle|OCI)`,
		`OracleException`),
	"IBM DB2": compilePatterns(
		`CLI Driver.*DB2`,
		`DB2 SQL error`,
		`\bdb2_\w+\(`,
		`SQLCODE[=:\d, -]+SQLSTATE`,
		`com\.ibm\.db2\.jcc`,
		`Zend_Db_(Adapter|Statement)_Db2_Exception`,
		`Pdo[./_\\]Ibm`,
		`DB2Exception`,
		`ibm_db_dbi\.ProgrammingError`),
	"SQLite": compilePatterns(
		`SQLite/JDBCDriver`,
		`SQLite.Exception`,
		`System.Data.SQLite.SQLiteException`,
		`Warning.*sqlite_.*`,
		`Warning.*SQLite3::`,
		`sqlite3.OperationalError`,
		`sqlite3.ProgrammingError`,
		`\[SQLITE_ERROR\]`,
		`(Microsoft|System)\.Data\.SQLite\.SQLiteException`,
		`SQLite error \d+:`,
		`SQLite3::SQLException`,
		`org\.sqlite\.JDBC`,
		`Pdo[./_\\]Sqlite`,
		`SQLiteException`,
		// Raw error text from the Go SQLite drivers (modernc.org/sqlite,
		// mattn/go-sqlite3). The generic phrases are anchored to the trailing colon
		// SQLite always emits so bare prose in a non-SQLite response cannot false-fire.
		`SQL logic error`,
		`unrecognized token:`,
		`no such column:`,
		`no such table:`,
		`no such function:`,
		`misuse of aggregate`,
		`near "[^"]*": syntax error`),
	"Sybase": compilePatterns(
		`(?i)Warning.*sybase.*`,
		`Sybase message`,
		`Sybase.*Server message.*`,
		`Warning.*?\Wsybase_`,
		`SybSQLException`,
		`Sybase\.Data\.AseClient`,
		`com\.sybase\.jdbc`),
	"MongoDB": compilePatterns(
		`MongoError`,
		`failed to connect to server .* on first connect`,
		`E11000 duplicate key error collection`,
		`collection .* already exists`,
		`\bdeadlock\b.*\bdetected\b`,
		`Failed to parse:.*'filter'.*`,
		`unknown operator: \$\w+`,
		`No array filter found for identifier.*in path.*`,
		`Cannot use.*as a query operator`,
		`Cannot do exclusion on path.*in inclusion projection`,
		`Path.*intersects with a project inclusion`,
		`Unrecognized expression '\$[^']*'`,
		`is not a valid hex number`,
		`Failed to parse document from.*: *unexpected character.*after document key`),
	"CouchDB": compilePatterns(
		`unauthorized to access or create database`,
		`no_db_file`,
		`document update conflict`,
		`invalid UTF-8 JSON`,
		`badmatch`),
	"Cassandra": compilePatterns(
		`Cassandra.*InvalidQueryException`,
		// Cassandra shares ANTLR grammar errors with many non-DB parsers, so
		// these require CQL-specific framing rather than the bare ANTLR text.
		`SyntaxError: line \d+:\d+ (mismatched input|no viable alternative at input)`,
		`InvalidRequestException`,
		`code=2000 \[Syntax error in CQL query\]`),
	"Redis": compilePatterns(
		`redis.*WRONGTYPE`,
		`redis.*syntax error`),
	"Elasticsearch": compilePatterns(
		`SearchPhaseExecutionException`,
		`QueryParsingException`,
		`ElasticsearchParseException`,
		`json_parse_exception`,
		`parsing_exception`,
		`x_content_parse_exception`),
	"DynamoDB": compilePatterns(
		`ValidationException`,
		`com.amazonaws.services.dynamodbv2.model.AmazonDynamoDBException`,
		`ProvisionedThroughputExceededException`),
	"HBase": compilePatterns(
		`org.apache.hadoop.hbase.DoNotRetryIOException`,
		`ERROR: org.apache.hadoop.hbase.MasterNotRunningException`,
		`org.apache.hadoop.hbase.regionserver.NoSuchColumnFamilyException`),
	"Neo4j": compilePatterns(
		`Neo.ClientError.Statement.SyntaxError`,
		`org.neo4j.driver.v1.exceptions.ClientException`,
		`org.neo4j.driver.v1.exceptions.DatabaseException`),

	"Informix": compilePatterns(
		`Warning.*?\Wifx_`,
		`Exception.*?Informix`,
		`Informix ODBC Driver`,
		`ODBC Informix driver`,
		`com\.informix\.jdbc`,
		`weblogic\.jdbc\.informix`,
		`Pdo[./_\\]Informix`,
		`IfxException`),
	"Firebird": compilePatterns(
		`Dynamic SQL Error`,
		`Warning.*?\Wibase_`,
		`org\.firebirdsql\.jdbc`,
		`Pdo[./_\\]Firebird`),
	"SAP MaxDB": compilePatterns(
		`SQL error.*?POS([0-9]+)`,
		`Warning.*?\Wmaxdb_`,
		`DriverSapDB`,
		`-3014.*?Invalid end of SQL statement`,
		`com\.sap\.dbtech\.jdbc`,
		`\[-3008\].*?: Invalid keyword or missing delimiter`),
	"Ingres": compilePatterns(
		`Warning.*?\Wingres_`,
		`Ingres SQLSTATE`,
		`Ingres\W.*?Driver`,
		`com\.ingres\.gcf\.jdbc`),
	"FrontBase": compilePatterns(
		`Exception (condition )?\d+\. Transaction rollback`,
		`com\.frontbase\.jdbc`,
		`Syntax error 1. Missing`,
		`(Semantic|Syntax) error [1-4]\d{2}\.`),
	"HSQLDB": compilePatterns(
		`Unexpected end of command in statement \[`,
		`Unexpected token.*?in statement \[`,
		`org\.hsqldb\.jdbc`),
	"H2": compilePatterns(
		`org\.h2\.jdbc`,
		`\[42000-192\]`),
	"MonetDB": compilePatterns(
		`![0-9]{5}![^\n]+(failed|unexpected|error|syntax|expected|violation|exception)`,
		`\[MonetDB\]\[ODBC Driver`,
		`nl\.cwi\.monetdb\.jdbc`),
	"Apache Derby": compilePatterns(
		`Syntax error: Encountered`,
		`org\.apache\.derby`,
		`ERROR 42X01`),
	"Vertica": compilePatterns(
		`, Sqlstate: (3F|42).{3}, (Routine|Hint|Position):`,
		`/vertica/Parser/scan`,
		`com\.vertica\.jdbc`,
		`org\.jkiss\.dbeaver\.ext\.vertica`,
		`com\.vertica\.dsi\.dataengine`),
	"Mckoi": compilePatterns(
		`com\.mckoi\.JDBCDriver`,
		`com\.mckoi\.database\.jdbc`,
		`&lt;REGEX_LITERAL&gt;`),
	"Presto": compilePatterns(
		`com\.facebook\.presto\.jdbc`,
		`io\.prestosql\.jdbc`,
		`com\.simba\.presto\.jdbc`,
		`UNION query has different number of fields: \d+, \d+`,
		`line \d+:\d+: mismatched input '[^']+'. Expecting:`),
	"Altibase": compilePatterns(
		`Altibase\.jdbc\.driver`),
	"MimerSQL": compilePatterns(
		`com\.mimer\.jdbc`,
		`Syntax error,[^\n]+assumed to mean`),
	"ClickHouse": compilePatterns(
		`Code: \d+. DB::Exception:`,
		`Syntax error: failed at position \d+`),
	"CrateDB": compilePatterns(
		`io\.crate\.client\.jdbc`),
	"Cache": compilePatterns(
		`encountered after end of query`,
		`A comparison operator is required here`),
	"Raima Database Manager": compilePatterns(
		`-10048: Syntax error`,
		`rdmStmtPrepare\(.+?\) returned`),
	"Virtuoso": compilePatterns(
		`SQ074: Line \d+:`,
		`SR185: Undefined procedure`,
		`SQ200: No table `,
		`Virtuoso S0002 Error`,
		`\[(Virtuoso Driver|Virtuoso iODBC Driver)\]\[Virtuoso Server\]`),
}

type DatabaseErrorMatch struct {
	DatabaseName string
	MatchStr     string
}

// nonRelationalEngines are the engines in DBMS_ERRORS that are not SQL
// databases. Everything else is treated as relational.
var nonRelationalEngines = map[string]bool{
	"MongoDB":       true,
	"CouchDB":       true,
	"Cassandra":     true,
	"Redis":         true,
	"Elasticsearch": true,
	"DynamoDB":      true,
	"HBase":         true,
	"Neo4j":         true,
}

// IsRelational reports whether the matched engine is a SQL database. It lets
// callers avoid substantiating a NoSQL-injection claim with a SQL error (or the
// reverse), which would mislabel the finding.
func (m *DatabaseErrorMatch) IsRelational() bool {
	if m == nil {
		return false
	}
	return !nonRelationalEngines[m.DatabaseName]
}

// gatedPattern pairs an error pattern with a literal substring every match of it
// must contain. Patterns starting with a class or assertion (\b, (?i), (\W|\A))
// get no literal-prefix skip from the regexp package and re-scan the whole
// response; a Contains check rejects those in one pass. Empty gate = always run.
type gatedPattern struct {
	dbms string
	re   *regexp.Regexp
	gate string
	fold bool
}

var gatedDBMSErrors = buildGatedPatterns()

func buildGatedPatterns() []gatedPattern {
	names := make([]string, 0, len(DBMS_ERRORS))
	for name := range DBMS_ERRORS {
		names = append(names, name)
	}
	// Deterministic order: map iteration made the reported engine vary between
	// runs for a response matching more than one vendor's patterns.
	sort.Strings(names)

	out := make([]gatedPattern, 0, 256)
	for _, name := range names {
		for _, re := range DBMS_ERRORS[name] {
			gate, fold := requiredLiteral(re.String())
			out = append(out, gatedPattern{dbms: name, re: re, gate: gate, fold: fold})
		}
	}
	return out
}

func SearchDatabaseErrors(text string) *DatabaseErrorMatch {
	folded := ""
	foldedBuilt := false
	for _, p := range gatedDBMSErrors {
		if p.gate != "" {
			haystack := text
			if p.fold {
				if !foldedBuilt {
					folded, foldedBuilt = caseFold(text), true
				}
				haystack = folded
			}
			if !strings.Contains(haystack, p.gate) {
				continue
			}
		}
		if matchStr := p.re.FindString(text); matchStr != "" {
			return &DatabaseErrorMatch{DatabaseName: p.dbms, MatchStr: matchStr}
		}
	}
	return nil
}

// caseFold lowercases text so that containment of an ASCII lowercase gate is
// implied by any case-insensitive regexp match. ToLower alone is not enough: the
// regexp package folds via unicode.SimpleFold, whose orbits for 's' and 'k'
// include U+017F and U+212A, and ToLower leaves U+017F unchanged.
func caseFold(text string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 'ſ':
			return 's'
		case 'K':
			return 'k'
		}
		return unicode.ToLower(r)
	}, text)
}

// requiredLiteral returns a substring that every match of the pattern must
// contain, together with whether the comparison has to be case-insensitive. It
// returns "" whenever no such substring can be proven, so a wrong answer here
// can only cost time, never a missed match.
func requiredLiteral(pattern string) (string, bool) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	lit, fold := mandatoryLiteral(re.Simplify())
	if len(lit) < 3 || !isASCII(lit) {
		return "", false
	}
	if fold {
		return strings.ToLower(lit), true
	}
	return lit, false
}

// mandatoryLiteral walks only the constructs that guarantee the literal appears
// in every match; anything optional or alternated yields "".
func mandatoryLiteral(re *syntax.Regexp) (string, bool) {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune), re.Flags&syntax.FoldCase != 0
	case syntax.OpCapture:
		return mandatoryLiteral(re.Sub[0])
	case syntax.OpPlus, syntax.OpRepeat:
		if re.Op == syntax.OpRepeat && re.Min == 0 {
			return "", false
		}
		return mandatoryLiteral(re.Sub[0])
	case syntax.OpConcat:
		best, bestFold := "", false
		for _, sub := range re.Sub {
			lit, fold := mandatoryLiteral(sub)
			if len(lit) > len(best) {
				best, bestFold = lit, fold
			}
		}
		return best, bestFold
	default:
		return "", false
	}
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
