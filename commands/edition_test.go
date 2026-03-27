package commands

import (
	"strings"
	"testing"

	"github.com/klurvio/sukko-cli/client"
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

func TestMergeEditionUsage(t *testing.T) {
	t.Parallel()

	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name        string
		dst         client.EditionUsage
		src         client.EditionUsage
		wantConns   *int
		wantShards  *int
		wantTenants *int
	}{
		{
			name:        "fills nil connections and shards from src",
			dst:         client.EditionUsage{Tenants: intPtr(5)},
			src:         client.EditionUsage{Connections: intPtr(1200), Shards: intPtr(2)},
			wantTenants: intPtr(5),
			wantConns:   intPtr(1200),
			wantShards:  intPtr(2),
		},
		{
			name:       "does not overwrite non-nil dst fields",
			dst:        client.EditionUsage{Connections: intPtr(999), Shards: intPtr(4)},
			src:        client.EditionUsage{Connections: intPtr(1200), Shards: intPtr(2)},
			wantConns:  intPtr(999),
			wantShards: intPtr(4),
		},
		{
			name:       "both nil — stays nil",
			dst:        client.EditionUsage{},
			src:        client.EditionUsage{},
			wantConns:  nil,
			wantShards: nil,
		},
		{
			name:       "src nil — dst unchanged",
			dst:        client.EditionUsage{Connections: intPtr(500)},
			src:        client.EditionUsage{},
			wantConns:  intPtr(500),
			wantShards: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mergeEditionUsage(&tt.dst, &tt.src)

			checkIntPtr(t, "Connections", tt.dst.Connections, tt.wantConns)
			checkIntPtr(t, "Shards", tt.dst.Shards, tt.wantShards)
			if tt.wantTenants != nil {
				checkIntPtr(t, "Tenants", tt.dst.Tenants, tt.wantTenants)
			}
		})
	}
}

func checkIntPtr(t *testing.T, name string, got, want *int) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if got == nil || want == nil {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if *got != *want {
		t.Errorf("%s: got %d, want %d", name, *got, *want)
	}
}
