package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// All four expected metric families.
var requiredMetrics = []string{
	"locksmith_acquires_total",
	"locksmith_releases_total",
	"locksmith_rejections_total",
	"locksmith_locks",
}

// TestMetrics verifies that all locksmith metrics are present in the scraping
// endpoint and that each counter has been incremented at least once.
func TestMetrics(t *testing.T) {
	startMetrics := fetchMetrics(t)

	// Acquire and release a lock to hit acquireCounter, releaseCounter, and lockGauge.
	c1, acquired := newClient(t)
	if err := c1.Acquire("metric-acquire-release"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for lock acquisition")
	}
	if err := c1.Release("metric-acquire-release"); err != nil {
		t.Fatalf("release: %v", err)
	}

	// Release a lock that was never acquired to hit rejectionCounter.
	c2, _ := newClient(t)
	if err := c2.Release("metric-rejection-lock"); err != nil {
		// This looks confusing but is correct. Locksmith will kill this connection, but the TCP write will succeed first.
		t.Fatalf("rejection trigger: %v", err)
	}

	endMetrics := fetchMetrics(t)
	for {

		missing := false
		for _, name := range requiredMetrics {
			if _, ok := endMetrics[name]; !ok {
				t.Logf("metric %q not found in scrape output", name)
				missing = true
			}
		}

		if missing {
			// If any metrics were missing, wait briefly and try again to allow for
			// any startup delays in the metrics subsystem.
			time.Sleep(500 * time.Millisecond)
			endMetrics = fetchMetrics(t)
			continue
		}

		// All metrics are present, proceed with validation.
		break
	}

	// Each counter must have incremented at least once.
	counterMetrics := []string{
		"locksmith_acquires_total",
		"locksmith_releases_total",
		"locksmith_rejections_total",
	}
	for _, name := range counterMetrics {
		end, _ := endMetrics[name]

		endVal := sumCounterValues(end)
		startVal := float64(0) // default to zero if metric was missing at start
		if start, ok := startMetrics[name]; ok {
			startVal = sumCounterValues(start) // override if found in initial scrape
		}

		if endVal <= startVal {
			t.Errorf("metric %q did not increment: start=%f end=%f", name, startVal, endVal)
		}
	}
}

func fetchMetrics(t *testing.T) map[string]*io_prometheus_client.MetricFamily {
	t.Helper()
	addr := fmt.Sprintf("http://%s:%d/metrics", locksmithHost, metricsPort)
	req, err := http.NewRequest(http.MethodGet, addr, nil)
	if err != nil {
		t.Fatalf("create metrics request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics endpoint status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}
	parser := expfmt.NewTextParser(model.LegacyValidation)
	metrics, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		t.Fatalf("parse metrics: %v", err)
	}
	return metrics
}

func sumCounterValues(mf *io_prometheus_client.MetricFamily) float64 {
	var total float64
	for _, m := range mf.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	return total
}
