package commands

import (
	"strings"
	"testing"
)

func TestEditionCmd_Registration(t *testing.T) {
	t.Parallel()

	subs := editionCmd.Commands()
	if len(subs) != 1 {
		t.Fatalf("have %d subcommands, want 1 (compare)", len(subs))
	}
	if subs[0].Name() != "compare" {
		t.Errorf("subcommand name = %q, want compare", subs[0].Name())
	}
}

func TestCapitalizeEdition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"community", "Community"},
		{"pro", "Pro"},
		{"enterprise", "Enterprise"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := capitalizeEdition(tt.input); got != tt.want {
			t.Errorf("capitalizeEdition(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEditionCompare_TableWidth(t *testing.T) {
	t.Parallel()

	// Verify all rows fit within 80 columns (NFR-005)
	// Format: "%-24s %-14s %-14s %-14s" = 24+14+14+14 = 66 chars max
	for _, row := range editionMatrix {
		if row.dimension == "" {
			continue
		}
		// Each column: value + padding to width
		lineLen := len(row.dimension)
		if lineLen > 24 {
			t.Errorf("dimension %q is %d chars, max 24 for 80-col table", row.dimension, lineLen)
		}
		for _, val := range []string{row.community, row.pro, row.enterprise} {
			if len(val) > 14 {
				t.Errorf("value %q is %d chars, max 14 for 80-col table", val, len(val))
			}
		}
	}
}

func TestEditionCompare_DimensionCount(t *testing.T) {
	t.Parallel()

	// Count non-separator rows (FR-014 specifies 13 dimensions)
	count := 0
	for _, row := range editionMatrix {
		if row.dimension != "" {
			count++
		}
	}
	if count != 13 {
		t.Errorf("editionMatrix has %d dimensions, want 13", count)
	}
}

func TestComparisonData_JSON(t *testing.T) {
	t.Parallel()

	data := comparisonData()
	editions, ok := data["editions"].([]map[string]any)
	if !ok {
		t.Fatal("editions field missing or wrong type")
	}
	if len(editions) != 3 {
		t.Fatalf("have %d editions, want 3", len(editions))
	}

	names := []string{"community", "pro", "enterprise"}
	for i, e := range editions {
		name, _ := e["name"].(string)
		if name != names[i] {
			t.Errorf("edition[%d] name = %q, want %q", i, name, names[i])
		}
	}

	url, _ := data["upgrade_url"].(string)
	if !strings.Contains(url, "sukko.dev") {
		t.Errorf("upgrade_url = %q, want sukko.dev URL", url)
	}
}
