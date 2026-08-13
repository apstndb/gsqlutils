package stmtkind

import "testing"

func TestDetectLexical_ExportData(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{
			name: "uppercase",
			sql: `EXPORT DATA OPTIONS (
  format = "CLOUD_SPANNER",
  table = "Account"
) AS
GRAPH FinGraph
CALL PageRank() YIELD node, score
RETURN node.id, score`,
		},
		{
			name: "lowercase",
			sql: `export data options (
  uri = "gs://bucket/output.csv",
  format = "csv"
) as
graph FinGraph
call PageRank() yield node, score
return node.id, score`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, err := DetectLexical(tt.sql)
			if err != nil {
				t.Fatalf("DetectLexical() error = %v", err)
			}
			if kind != StatementKindExportData {
				t.Fatalf("DetectLexical() = %v, want %v", kind, StatementKindExportData)
			}
			if !kind.IsExportData() {
				t.Error("IsExportData() = false, want true")
			}
			if !kind.IsExecuteSQLCompatible() {
				t.Error("IsExecuteSQLCompatible() = false, want true")
			}
			if kind.IsQuery() {
				t.Error("IsQuery() = true, want false")
			}
			if got := kind.String(); got != "EXPORT DATA" {
				t.Errorf("String() = %q, want %q", got, "EXPORT DATA")
			}
		})
	}
}
