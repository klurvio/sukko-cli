package compose

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		projectDir  string
		composeFile string
		wantErr     bool
	}{
		{"valid", "/tmp/project", "/tmp/project/.sukko/docker-compose.yml", false},
		{"empty_dir", "", "/tmp/compose.yml", true},
		{"empty_file", "/tmp/project", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := NewManager(tt.projectDir, tt.composeFile)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m == nil {
				t.Fatal("expected non-nil manager")
			}
			if m.projectDir != tt.projectDir {
				t.Errorf("projectDir = %q, want %q", m.projectDir, tt.projectDir)
			}
			if m.composeFile != tt.composeFile {
				t.Errorf("composeFile = %q, want %q", m.composeFile, tt.composeFile)
			}
		})
	}
}

func TestComposeFileContent(t *testing.T) {
	t.Parallel()

	if len(ComposeFileContent) == 0 {
		t.Fatal("embedded ComposeFileContent is empty")
	}
}
