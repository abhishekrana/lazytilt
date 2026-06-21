package tilt

import (
	"os"
	"path/filepath"
	"testing"
)

func loadView(t *testing.T, name string) *View {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	v, err := ParseView(b)
	if err != nil {
		t.Fatalf("parse view: %v", err)
	}
	return v
}

func find(v *View, name string) *UIResource {
	for i := range v.UIResources {
		if v.UIResources[i].Name() == name {
			return &v.UIResources[i]
		}
	}
	return nil
}

func TestDecodeK8s(t *testing.T) {
	v := loadView(t, "view_k8s.json")

	if got := v.Version(); got != "0.35.0" {
		t.Errorf("version = %q, want 0.35.0", got)
	}

	// Resources sort by Tilt order.
	res := v.Resources()
	if res[0].Name() != "(Tiltfile)" {
		t.Errorf("first resource = %q, want (Tiltfile)", res[0].Name())
	}

	api := find(v, "api")
	if api.Backend() != "k8s" {
		t.Errorf("api backend = %q, want k8s", api.Backend())
	}
	if api.State() != StatusOK {
		t.Errorf("api state = %v, want OK", api.State())
	}
	if got := api.RuntimeLine(); got != "pod api-pod-1 · Running" {
		t.Errorf("api runtime line = %q", got)
	}

	worker := find(v, "worker")
	if worker.State() != StatusError {
		t.Errorf("worker state = %v, want Error", worker.State())
	}
	if worker.LastError() == "" {
		t.Error("worker should have a build error")
	}
	if got := worker.RuntimeLine(); got != "pod pending" {
		t.Errorf("worker runtime = %q, want 'pod pending'", got)
	}

	db := find(v, "db")
	if got := db.RuntimeLine(); got != "pod db-pod-0 · Running · restarts 2" {
		t.Errorf("db runtime = %q", got)
	}

	batch := find(v, "batch-job")
	if !batch.IsDisabled() || batch.State() != StatusDisabled {
		t.Errorf("batch-job should be disabled, got state %v", batch.State())
	}

	// Counts exclude the disabled resource: 1 error, 3 ok, 4 total.
	errs, ok, total := v.Counts()
	if errs != 1 || ok != 3 || total != 4 {
		t.Errorf("counts = (%d,%d,%d), want (1,3,4)", errs, ok, total)
	}
}

func TestDecodeCompose(t *testing.T) {
	v := loadView(t, "view_compose.json")

	web := find(v, "web")
	if web.Backend() != "compose" {
		t.Errorf("web backend = %q, want compose", web.Backend())
	}
	if got := web.RuntimeLine(); got != "container · healthy" {
		t.Errorf("web runtime = %q", got)
	}

	metrics := find(v, "metrics")
	if len(metrics.Status.EndpointLinks) != 1 || metrics.Status.EndpointLinks[0].URL != "http://localhost:8080/" {
		t.Errorf("metrics endpoint not decoded: %+v", metrics.Status.EndpointLinks)
	}

	queue := find(v, "queue")
	if queue.State() != StatusNone {
		t.Errorf("queue state = %v, want None (idle)", queue.State())
	}
}

func TestLogAssembly(t *testing.T) {
	v := loadView(t, "view_k8s.json")

	// Per-resource logs filter by the span's manifest.
	apiLines := Lines(v.LogList.SegmentsFor("api"))
	if len(apiLines) != 1 || apiLines[0] != "listening on :8080" {
		t.Errorf("api logs = %v", apiLines)
	}

	workerSegs := v.LogList.SegmentsFor("worker")
	if len(workerSegs) != 2 {
		t.Fatalf("worker segments = %d, want 2", len(workerSegs))
	}
	for _, s := range workerSegs {
		if s.Level != "ERROR" {
			t.Errorf("worker log level = %q, want ERROR", s.Level)
		}
	}

	// Global (empty span) output is not attributed to any resource.
	if got := v.LogList.ManifestOf(""); got != "" {
		t.Errorf("empty span manifest = %q, want empty", got)
	}
}
