package bigquery

import (
	"strings"
	"testing"
	"time"
)

func TestTranslateBQtoDuck_PreservesLiterals(t *testing.T) {
	datasets := map[string]bool{"retail_raw": true, "retail_staging": true}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "decimal literals are not identifiers",
			in:   "INSERT INTO retail_raw.orders (id, price) VALUES (1, 24.99)",
			want: "INSERT INTO retail_raw__orders (id, price) VALUES (1, 24.99)",
		},
		{
			name: "exponent literals survive",
			in:   "SELECT 1.5e-9 AS tiny FROM retail_raw.orders",
			want: "SELECT 1.5e-9 AS tiny FROM retail_raw__orders",
		},
		{
			name: "dots inside string literals are untouched",
			in:   "SELECT * FROM retail_raw.customers WHERE email = 'ada.okonkwo@example.com'",
			want: "SELECT * FROM retail_raw__customers WHERE email = 'ada.okonkwo@example.com'",
		},
		{
			name: "escaped quote inside a literal does not end it",
			in:   `SELECT 'o''brien.smith@example.com' AS who`,
			want: `SELECT 'o''brien.smith@example.com' AS who`,
		},
		{
			name: "table aliases keep their qualification",
			in:   "SELECT o.order_id FROM retail_raw.orders AS o",
			want: "SELECT o.order_id FROM retail_raw__orders AS o",
		},
		{
			name: "fully qualified references drop the project",
			in:   "SELECT * FROM retail-analytics-local.retail_raw.orders",
			want: "SELECT * FROM retail_raw__orders",
		},
		{
			name: "backtick quoting is normalised",
			in:   "SELECT * FROM `retail-analytics-local`.`retail_raw`.`orders`",
			want: "SELECT * FROM retail_raw__orders",
		},
		{
			name: "backtick quoting of a whole chain is normalised",
			in:   "SELECT * FROM `retail-analytics-local.retail_raw.orders`",
			want: "SELECT * FROM retail_raw__orders",
		},
		{
			name: "column selected off a qualified relation",
			in:   "SELECT retail_raw.orders.order_id FROM retail_raw.orders",
			want: "SELECT retail_raw__orders.order_id FROM retail_raw__orders",
		},
		{
			name: "comments are copied verbatim",
			in:   "-- price is 24.99 for a.b\nSELECT 1",
			want: "-- price is 24.99 for a.b\nSELECT 1",
		},
		{
			name: "block comments are copied verbatim",
			in:   "/* keeps a.b and 1.5 */ SELECT * FROM retail_raw.orders",
			want: "/* keeps a.b and 1.5 */ SELECT * FROM retail_raw__orders",
		},
		{
			name: "CURRENT_TIMESTAMP loses its parens",
			in:   "SELECT CURRENT_TIMESTAMP() AS now",
			want: "SELECT CURRENT_TIMESTAMP AS now",
		},
		{
			name: "arithmetic on hyphenated names is left alone",
			in:   "SELECT a-b FROM retail_raw.orders",
			want: "SELECT a-b FROM retail_raw__orders",
		},
		{
			name: "create table as select rewrites both relations",
			in:   "create or replace table retail_staging.stg_orders as (select * from retail_raw.orders)",
			want: "create or replace table retail_staging__stg_orders as (select * from retail_raw__orders)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := translateBQtoDuckWithDatasets(tc.in, datasets)
			if got != tc.want {
				t.Errorf("translate mismatch\n in:   %s\n got:  %s\n want: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestTranslateBQtoDuck_UnknownDatasetLeavesTwoPartChains(t *testing.T) {
	// Without a dataset registry a two-part chain is far more likely to be
	// alias.column than dataset.table, so it must not be collapsed.
	got := translateBQtoDuckWithDatasets("SELECT o.order_id FROM orders AS o", nil)
	if got != "SELECT o.order_id FROM orders AS o" {
		t.Errorf("unexpected rewrite: %s", got)
	}
}

func TestTranslateBQtoDuck_UnknownDatasetStillResolvesThreePartChains(t *testing.T) {
	// A three-part chain is unambiguously project.dataset.table in BigQuery, so
	// it stays resolvable even when the dataset registry is empty (e.g. after a
	// restart, when only the DuckDB file survives).
	got := translateBQtoDuckWithDatasets("SELECT * FROM proj.ds.tbl", nil)
	if got != "SELECT * FROM ds__tbl" {
		t.Errorf("got %q, want %q", got, "SELECT * FROM ds__tbl")
	}
}

func TestDuckToBQType(t *testing.T) {
	tests := map[string]string{
		"BIGINT":                   "INT64",
		"INTEGER":                  "INT64",
		"HUGEINT":                  "INT64",
		"DOUBLE":                   "FLOAT64",
		"DECIMAL(38,9)":            "NUMERIC",
		"BOOLEAN":                  "BOOL",
		"VARCHAR":                  "STRING",
		"BLOB":                     "BYTES",
		"DATE":                     "DATE",
		"TIME":                     "TIME",
		"TIMESTAMP":                "DATETIME",
		"TIMESTAMP WITH TIME ZONE": "TIMESTAMP",
		"JSON":                     "JSON",
		"VARCHAR[]":                "STRING",
		"STRUCT(a INTEGER)":        "STRING",
		"SOMETHING_NEW":            "STRING",
	}
	for in, want := range tests {
		if got := duckToBQType(in); got != want {
			t.Errorf("duckToBQType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitDuckTableName(t *testing.T) {
	dataset, table, ok := splitDuckTableName("retail_raw__orders")
	if !ok || dataset != "retail_raw" || table != "orders" {
		t.Errorf("got (%q, %q, %v), want (retail_raw, orders, true)", dataset, table, ok)
	}
	if _, _, ok := splitDuckTableName("orders"); ok {
		t.Error("a name without a dataset prefix must not split")
	}
}

func TestFormatCell(t *testing.T) {
	stamp := time.Date(2026, 7, 1, 12, 30, 0, 0, time.UTC)

	if got := formatCell(nil, "STRING"); got != nil {
		t.Errorf("nil must stay null, got %v", got)
	}
	if got := formatCell(stamp, "DATE"); got != "2026-07-01" {
		t.Errorf("DATE = %v, want 2026-07-01", got)
	}
	if got := formatCell(stamp, "DATETIME"); got != "2026-07-01T12:30:00" {
		t.Errorf("DATETIME = %v, want 2026-07-01T12:30:00", got)
	}
	// BigQuery reports TIMESTAMP as epoch seconds.
	if got := formatCell(stamp, "TIMESTAMP"); !strings.HasPrefix(got.(string), "1782") {
		t.Errorf("TIMESTAMP = %v, want epoch seconds", got)
	}
	if got := formatCell(24.99, "FLOAT64"); got != "24.99" {
		t.Errorf("FLOAT64 = %v, want 24.99", got)
	}
	if got := formatCell(true, "BOOL"); got != "true" {
		t.Errorf("BOOL = %v, want true", got)
	}
}
