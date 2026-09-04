package bigquery

import (
	"testing"
)

// This is the check that tells a platform whether BigQuery actually works on
// it. Where CGO is unavailable the backend degrades to a mock that answers
// every query with zero rows and no error, which is the worst possible failure
// — so a build that claims DuckDB support has to prove it executes SQL.
//
// It skips rather than fails without CGO, so the Linux, macOS and Windows
// builds can all run the same suite and only the CGO ones assert.
func TestDuckDBBackendExecutesRealSQL(t *testing.T) {
	t.Setenv("MINISKY_BQ_BACKEND", "duckdb")
	// The backend opens ~/.minisky/data/bigquery.duckdb, and DuckDB allows one
	// writer per file — so without its own home this test fails whenever a
	// MiniSky daemon happens to be running, and would otherwise write into the
	// developer's real warehouse.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	backend := NewDuckDBBackend()
	if !backend.Enabled() {
		t.Skip("built without CGO: BigQuery falls back to the empty-result mock")
	}

	// The casts are deliberate: a bare 24.99 is NUMERIC in DuckDB, not a float,
	// so an uncast literal would be asserting the driver's decimal type rather
	// than that the backend works.
	result, err := backend.ExecuteQueryWithSchema(
		`SELECT 42::BIGINT AS answer, 24.99::DOUBLE AS price, ` +
			`DATE '2026-07-01' AS day, 'ada.okonkwo@example.com' AS email`)
	if err != nil {
		t.Fatalf("query failed on a CGO build, so DuckDB is not usable here: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(result.Rows))
	}

	row := result.Rows[0]
	if got := row["answer"]; got != int64(42) {
		t.Errorf("answer = %v (%T), want 42", got, got)
	}
	// The value the SQL translator used to destroy by rewriting 24.99 to 24__99.
	if got := row["price"]; got != 24.99 {
		t.Errorf("price = %v (%T), want 24.99", got, got)
	}
	if got := row["email"]; got != "ada.okonkwo@example.com" {
		t.Errorf("email = %v, want the address intact", got)
	}

	// Typed columns are what separate a real backend from the mock.
	types := map[string]string{}
	for _, f := range result.Columns {
		types[f.Name] = f.Type
	}
	if types["answer"] != "INT64" || types["price"] != "FLOAT64" || types["day"] != "DATE" {
		t.Errorf("column types = %v; a real backend reports them, the mock says STRING", types)
	}
}
