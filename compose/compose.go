// Package compose provides Docker Compose integration for local development.
package compose

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

//go:embed docker-compose.yml
var ComposeFileContent []byte //nolint:revive // exported embedded content used by commands package

const defaultLogTailLines = "100"

// WriteComposeFile writes the embedded docker-compose.yml to the given path.
func WriteComposeFile(path string) error {
	if err := os.WriteFile(path, ComposeFileContent, 0o644); err != nil { //nolint:gosec // G306: compose file is not sensitive
		return fmt.Errorf("write compose file: %w", err)
	}
	return nil
}

// Manager wraps Docker Compose CLI operations.
type Manager struct {
	projectDir  string
	composeFile string
}

// NewManager creates a Manager for the given project directory and compose file path.
// Returns an error if projectDir or composeFile is empty.
func NewManager(projectDir, composeFile string) (*Manager, error) {
	if projectDir == "" {
		return nil, errors.New("project directory must not be empty")
	}
	if composeFile == "" {
		return nil, errors.New("compose file path must not be empty")
	}
	return &Manager{projectDir: projectDir, composeFile: composeFile}, nil
}

// composeArgs returns the base docker compose args with the -f flag.
func (m *Manager) composeArgs() []string {
	return []string{"compose", "-f", m.composeFile}
}

// ServiceStatus represents the status of a Docker Compose service.
type ServiceStatus struct {
	Name    string `json:"Name"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Service string `json:"Service"`
	Status  string `json:"Status"`
}

// Up starts services with the given profiles and environment overrides.
// If pull is true, images are always pulled before starting (--pull always).
func (m *Manager) Up(ctx context.Context, profiles []string, envOverrides map[string]string, pull bool) error {
	args := m.composeArgs()
	for _, p := range profiles {
		args = append(args, "--profile", p)
	}
	args = append(args, "up", "-d")
	if pull {
		args = append(args, "--pull", "always")
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: args built from fixed strings and validated profile names
	cmd.Dir = m.projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	env := os.Environ()
	for k, v := range envOverrides {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	return nil
}

// Down stops and removes services.
func (m *Manager) Down(ctx context.Context, removeVolumes bool) error {
	args := m.composeArgs()
	args = append(args, "down")
	if removeVolumes {
		args = append(args, "-v")
	}

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: args built from fixed strings
	cmd.Dir = m.projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}
	return nil
}

// Status returns the status of all services.
func (m *Manager) Status(ctx context.Context) ([]ServiceStatus, error) {
	args := m.composeArgs()
	args = append(args, "ps", "--format", "json")

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: args built from fixed strings
	cmd.Dir = m.projectDir

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}

	if strings.TrimSpace(string(out)) == "" {
		return nil, nil
	}

	// docker compose ps --format json outputs one JSON object per line
	var services []ServiceStatus
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var svc ServiceStatus
		if err := json.Unmarshal([]byte(line), &svc); err != nil {
			// docker compose may emit non-JSON preamble or progress lines; silently
			// skipping them is safe because valid service entries are always JSON objects.
			continue
		}
		services = append(services, svc)
	}

	return services, nil
}

// Logs streams logs from the specified services. If services is empty, tails all.
func (m *Manager) Logs(ctx context.Context, services []string, follow bool) error {
	args := m.composeArgs()
	args = append(args, "logs")
	if follow {
		args = append(args, "-f")
	}
	args = append(args, "--tail", defaultLogTailLines)
	args = append(args, services...)

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: args built from fixed strings
	cmd.Dir = m.projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose logs: %w", err)
	}
	return nil
}

// IsRunning returns true if the Docker Compose project has running services.
func (m *Manager) IsRunning(ctx context.Context) bool {
	args := m.composeArgs()
	args = append(args, "ps", "-q")

	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204: args built from fixed strings
	cmd.Dir = m.projectDir

	// err ignored: docker compose ps failure (daemon unreachable, not installed) is treated as "not running"
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
