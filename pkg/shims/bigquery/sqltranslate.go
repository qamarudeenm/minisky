package bigquery

import (
	"log"
	"regexp"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// BigQuery → DuckDB SQL translation
//
// DuckDB has a flat namespace, so a BigQuery relation `project.dataset.table`
// is materialised as the single identifier "dataset__table". Rewriting those
// references used to be a pair of regex replacements over the whole statement,
// which also mangled anything else containing a dot: numeric literals
// (24.99 → 24__99), string contents ('a.b@c.com' → 'a__b@c__com') and
// qualified column references (o.order_id → o__order_id).
//
// The scanner below rewrites only what is actually a relation reference. String
// literals, quoted identifiers, comments and numbers are copied through
// verbatim, and a two-part chain is only joined when its first segment is a
// dataset the emulator knows about — so table aliases survive untouched.
// ─────────────────────────────────────────────────────────────────────────────

// bqToDuckTypeMap maps BigQuery field types to DuckDB equivalents.
var bqToDuckTypeMap = map[string]string{
	"STRING":     "VARCHAR",
	"BYTES":      "BLOB",
	"INTEGER":    "BIGINT",
	"INT64":      "BIGINT",
	"FLOAT":      "DOUBLE",
	"FLOAT64":    "DOUBLE",
	"NUMERIC":    "DECIMAL(38,9)",
	"BIGNUMERIC": "DECIMAL(38,9)",
	"BOOLEAN":    "BOOLEAN",
	"BOOL":       "BOOLEAN",
	"TIMESTAMP":  "TIMESTAMPTZ",
	"DATE":       "DATE",
	"TIME":       "TIME",
	"DATETIME":   "TIMESTAMP",
	"GEOGRAPHY":  "VARCHAR", // approximate — DuckDB lacks native GEOGRAPHY
	"JSON":       "JSON",
	"RECORD":     "STRUCT", // nested — requires recursive handling
	"STRUCT":     "STRUCT",
}

// duckTypeParams strips the parameter list from a DuckDB type name,
// e.g. DECIMAL(38,9) → DECIMAL.
var duckTypeParams = regexp.MustCompile(`\(.*\)`)

// duckToBQType maps a DuckDB column type back to the BigQuery type name the
// REST API reports, so clients receive real types instead of everything STRING.
func duckToBQType(duckType string) string {
	name := strings.ToUpper(strings.TrimSpace(duckTypeParams.ReplaceAllString(duckType, "")))

	switch {
	case strings.HasSuffix(name, "[]"), strings.HasPrefix(name, "LIST"),
		strings.HasPrefix(name, "STRUCT"), strings.HasPrefix(name, "MAP"),
		strings.HasPrefix(name, "UNION"):
		// Nested types are surfaced as strings: the shim does not yet emit the
		// nested `fields` a real RECORD carries.
		return "STRING"
	}

	switch name {
	case "BIGINT", "INT8", "LONG", "INTEGER", "INT", "INT4", "SIGNED",
		"SMALLINT", "INT2", "SHORT", "TINYINT", "INT1", "HUGEINT",
		"UBIGINT", "UINTEGER", "USMALLINT", "UTINYINT":
		return "INT64"
	case "DOUBLE", "FLOAT8", "FLOAT", "FLOAT4", "REAL":
		return "FLOAT64"
	case "DECIMAL", "NUMERIC":
		return "NUMERIC"
	case "BOOLEAN", "BOOL", "LOGICAL":
		return "BOOL"
	case "BLOB", "BYTEA", "BINARY", "VARBINARY":
		return "BYTES"
	case "DATE":
		return "DATE"
	case "TIME":
		return "TIME"
	case "TIMESTAMPTZ", "TIMESTAMP WITH TIME ZONE":
		return "TIMESTAMP"
	case "TIMESTAMP", "DATETIME", "TIMESTAMP_S", "TIMESTAMP_MS", "TIMESTAMP_NS":
		return "DATETIME"
	case "JSON":
		return "JSON"
	case "UUID", "VARCHAR", "CHAR", "BPCHAR", "TEXT", "STRING":
		return "STRING"
	default:
		return "STRING"
	}
}

// duckTableName is the DuckDB identifier for a BigQuery table.
func duckTableName(dataset, table string) string { return dataset + "__" + table }

// splitDuckTableName reverses duckTableName. ok is false when the identifier
// does not carry a dataset prefix.
func splitDuckTableName(name string) (dataset, table string, ok bool) {
	idx := strings.Index(name, "__")
	if idx <= 0 || idx+2 >= len(name) {
		return "", "", false
	}
	return name[:idx], name[idx+2:], true
}

// translateBQtoDuck converts a BigQuery statement to its DuckDB equivalent
// without a set of known datasets: two-part references are then left alone and
// only fully-qualified project.dataset.table references are rewritten.
func translateBQtoDuck(bqSQL string) string {
	return translateBQtoDuckWithDatasets(bqSQL, nil)
}

// translateBQtoDuckWithDatasets is translateBQtoDuck with the dataset names the
// emulator currently knows, which is what makes `dataset.table` distinguishable
// from `alias.column`.
func translateBQtoDuckWithDatasets(bqSQL string, datasets map[string]bool) string {
	s := normaliseBacktickIdentifiers(bqSQL)
	s = strings.ReplaceAll(s, "CURRENT_TIMESTAMP()", "CURRENT_TIMESTAMP")

	// Full translation of these needs a real SQL parser; warn rather than
	// silently produce a different result.
	if strings.Contains(s, "TIMESTAMP_TRUNC") {
		log.Printf("[DuckDBBackend] WARN: TIMESTAMP_TRUNC requires manual translation — result may vary")
	}
	if strings.Contains(s, "SAFE_DIVIDE") {
		log.Printf("[DuckDBBackend] WARN: SAFE_DIVIDE not auto-translated — consider rewriting query")
	}

	return rewriteRelations(s, datasets)
}

// backtickChain matches a backtick-quoted identifier or identifier chain.
var backtickChain = regexp.MustCompile("`([A-Za-z0-9_.\\-]+)`")

// normaliseBacktickIdentifiers turns BigQuery's backtick quoting into bare
// identifiers so the scanner sees a single uniform form. `p`.`d`.`t` and
// `p.d.t` both become p.d.t; anything that is not a plain identifier chain
// (spaces, punctuation) keeps its quotes and is treated as a quoted literal.
func normaliseBacktickIdentifiers(sql string) string {
	return backtickChain.ReplaceAllString(sql, "$1")
}

// rewriteRelations walks the statement and rewrites relation references only.
func rewriteRelations(sql string, datasets map[string]bool) string {
	var out strings.Builder
	out.Grow(len(sql))

	for i := 0; i < len(sql); {
		c := sql[i]

		switch {
		// ── string literal / quoted identifier ──────────────────────────────
		case c == '\'' || c == '"' || c == '`':
			end := scanQuoted(sql, i)
			out.WriteString(sql[i:end])
			i = end

		// ── line comment ────────────────────────────────────────────────────
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			end := strings.IndexByte(sql[i:], '\n')
			if end < 0 {
				out.WriteString(sql[i:])
				i = len(sql)
			} else {
				out.WriteString(sql[i : i+end+1])
				i += end + 1
			}

		// ── block comment ───────────────────────────────────────────────────
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				out.WriteString(sql[i:])
				i = len(sql)
			} else {
				out.WriteString(sql[i : i+2+end+2])
				i += 2 + end + 2
			}

		// ── numeric literal: 24.99 must never become 24__99 ──────────────────
		case isDigit(c):
			end := scanNumber(sql, i)
			out.WriteString(sql[i:end])
			i = end

		// ── identifier, possibly the head of a relation chain ────────────────
		case isIdentStart(c):
			segments, end := scanIdentifierChain(sql, i)
			out.WriteString(joinRelation(segments, datasets))
			i = end

		default:
			out.WriteByte(c)
			i++
		}
	}

	return out.String()
}

// scanQuoted returns the index just past the closing quote of the literal
// starting at i. Doubled quotes (” or "") and backslash escapes are consumed
// as part of the literal.
func scanQuoted(sql string, i int) int {
	quote := sql[i]
	j := i + 1
	for j < len(sql) {
		switch {
		case sql[j] == '\\' && quote == '\'' && j+1 < len(sql):
			j += 2
		case sql[j] == quote && j+1 < len(sql) && sql[j+1] == quote:
			j += 2
		case sql[j] == quote:
			return j + 1
		default:
			j++
		}
	}
	return len(sql) // unterminated — copy the remainder verbatim
}

// scanNumber returns the index just past a numeric literal, including its
// decimal point and exponent.
func scanNumber(sql string, i int) int {
	j := i
	for j < len(sql) && (isDigit(sql[j]) || sql[j] == '.') {
		j++
	}
	if j < len(sql) && (sql[j] == 'e' || sql[j] == 'E') {
		k := j + 1
		if k < len(sql) && (sql[k] == '+' || sql[k] == '-') {
			k++
		}
		if k < len(sql) && isDigit(sql[k]) {
			for k < len(sql) && isDigit(sql[k]) {
				k++
			}
			j = k
		}
	}
	return j
}

// scanIdentifierChain reads ident[.ident]* starting at i and returns its
// segments plus the index just past the chain. Hyphens are absorbed into a
// segment because GCP project ids contain them; that is harmless for segments
// that are not rewritten, since they are emitted verbatim.
func scanIdentifierChain(sql string, i int) ([]string, int) {
	var segments []string
	j := i

	for {
		start := j
		for j < len(sql) && isIdentChar(sql[j]) {
			j++
		}
		// A trailing hyphen belongs to the following operator, not the name.
		for j > start && sql[j-1] == '-' {
			j--
		}
		if j == start {
			break
		}
		segments = append(segments, sql[start:j])

		if j < len(sql) && sql[j] == '.' && j+1 < len(sql) && isIdentStart(sql[j+1]) {
			j++ // consume the dot and read the next segment
			continue
		}
		break
	}

	return segments, j
}

// joinRelation renders a scanned chain, collapsing dataset.table into
// dataset__table when the chain actually references a relation.
func joinRelation(segments []string, datasets map[string]bool) string {
	if len(segments) < 2 {
		return strings.Join(segments, ".")
	}

	// Preferred: the first segment that names a known dataset and is followed
	// by a table. Handles dataset.table, project.dataset.table and
	// project.dataset.table.column alike.
	for idx := 0; idx+1 < len(segments); idx++ {
		if datasets[segments[idx]] {
			collapsed := append([]string{duckTableName(segments[idx], segments[idx+1])}, segments[idx+2:]...)
			return strings.Join(collapsed, ".")
		}
	}

	// No known dataset: a three-part chain is unambiguously
	// project.dataset.table in BigQuery, so it is still safe to collapse.
	// A two-part chain is left alone — it is far more likely to be alias.column.
	if len(segments) >= 3 {
		collapsed := append([]string{duckTableName(segments[1], segments[2])}, segments[3:]...)
		return strings.Join(collapsed, ".")
	}

	return strings.Join(segments, ".")
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentChar(c byte) bool  { return isIdentStart(c) || isDigit(c) || c == '-' }

// ─────────────────────────────────────────────────────────────────────────────
// Backend result types (shared by the CGO and non-CGO builds)
// ─────────────────────────────────────────────────────────────────────────────

// QueryColumn is one column of a query result, typed as BigQuery reports it.
type QueryColumn struct {
	Name string
	Type string
}

// QueryResult is an ordered query result. Column order matters: the REST
// response renders each row as a positional list against this schema.
type QueryResult struct {
	Columns []QueryColumn
	Rows    []map[string]interface{}
}

// BackendTable describes a table materialised in DuckDB.
type BackendTable struct {
	Dataset string
	Table   string
	Schema  *TableSchema
}
