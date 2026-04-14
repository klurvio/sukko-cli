package compose

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	healthPollInterval  = 2 * time.Second
	healthClientTimeout = 5 * time.Second
)

// HealthTarget represents a service to check health for.
type HealthTarget struct {
	Name string
	URL  string
}

// WaitForHealth polls health endpoints until all return 200 or the context is canceled.
// Checks all services concurrently on each poll cycle and displays live progress.
func WaitForHealth(ctx context.Context, w io.Writer, targets []HealthTarget, timeout time.Duration) error {
	if len(targets) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{Timeout: healthClientTimeout}

	type serviceState struct {
		target  HealthTarget
		healthy bool
		elapsed time.Duration
	}

	states := make([]serviceState, len(targets))
	for i, t := range targets {
		states[i] = serviceState{target: t}
	}

	start := time.Now()
	ticker := time.NewTicker(healthPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			names := make([]string, 0, len(states))
			for _, s := range states {
				if !s.healthy {
					names = append(names, s.target.Name)
				}
			}
			return fmt.Errorf("health check timeout after %s: services still unhealthy: %v",
				timeout.Round(time.Second), names)
		case <-ticker.C:
			// Check all pending services concurrently
			var wg sync.WaitGroup
			for i := range states {
				if states[i].healthy {
					continue
				}
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, states[idx].target.URL, http.NoBody)
					if err != nil {
						return
					}
					resp, err := client.Do(req)
					if err != nil {
						return
					}
					_ = resp.Body.Close()
					if resp.StatusCode == http.StatusOK {
						states[idx].healthy = true
						states[idx].elapsed = time.Since(start)
					}
				}(i)
			}
			wg.Wait()

			// Print status for all services
			allHealthy := true
			for _, s := range states {
				if s.healthy {
					_, _ = fmt.Fprintf(w, "  %-16s healthy (%s)\n", s.target.Name+":", s.elapsed.Round(time.Second))
				} else {
					_, _ = fmt.Fprintf(w, "  %-16s waiting... (%s)\n", s.target.Name+":", time.Since(start).Round(time.Second))
					allHealthy = false
				}
			}

			if allHealthy {
				return nil
			}
		}
	}
}
