package tilt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSegmentsForWorkload(t *testing.T) {
	// Pod log spans are "pod:<manifest>:<podName>"; a workload's pods are
	// "<workload>-<hash/ordinal>". Match by the "pod:<manifest>:<workload>-" prefix.
	ll := LogList{Segments: []LogSegment{
		{SpanID: "pod:apps:web-abc-1", Text: "web 1\n"},
		{SpanID: "pod:apps:worker-def-2", Text: "worker\n"},
		{SpanID: "pod:apps:web-abc-1", Text: "web 2\n"},
		{SpanID: "monitor:apps:web-abc-1", Text: "event\n"}, // not a pod-log span
	}}
	got := ll.SegmentsForWorkload("apps", "web")
	if len(got) != 2 {
		t.Fatalf("got %d segments, want 2", len(got))
	}
	for _, s := range got {
		if !strings.HasPrefix(s.Text, "web ") {
			t.Errorf("unexpected segment %q", s.Text)
		}
	}
	if n := len(ll.SegmentsForWorkload("other", "web")); n != 0 {
		t.Errorf("cross-manifest match returned %d, want 0", n)
	}
}

func TestPodNameForWorkload(t *testing.T) {
	// Pod names come from the log span IDs, keyed "pod:<manifest>:<podName>".
	ll := LogList{Spans: map[string]LogSpan{
		"pod:apps:web-abc-1":    {ManifestName: "apps"},
		"pod:apps:worker-def-2": {ManifestName: "apps"},
		"build:apps:web":        {ManifestName: "apps"}, // not a pod span
		"pod:other:web-xyz-9":   {ManifestName: "other"},
	}}
	if got := ll.PodNameForWorkload("apps", "web"); got != "web-abc-1" {
		t.Errorf("PodNameForWorkload(apps, web) = %q, want %q", got, "web-abc-1")
	}
	if got := ll.PodNameForWorkload("apps", "db"); got != "" {
		t.Errorf("no-logs workload = %q, want empty", got)
	}
	// "web-" prefix must not leak across manifests.
	if got := ll.PodNameForWorkload("apps", "web-xyz"); got != "" {
		t.Errorf("cross-manifest/partial match = %q, want empty", got)
	}
}

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

func TestWorkloads(t *testing.T) {
	// helm_resource bundles a release: DisplayNames lists every object as
	// "<name>:<kind>". Workloads keeps only deployable kinds, sorted; glue is dropped.
	r := &UIResource{Status: UIResourceStatus{K8sResourceInfo: &K8sResourceInfo{
		DisplayNames: []string{
			"data-hub:deployment", "auth-service:service", "minio:secret",
			"postgresql:statefulset", "auth-service:deployment", "migrate:job",
		},
	}}}
	got := r.Workloads()
	want := []string{"auth-service", "data-hub", "migrate", "postgresql"}
	if len(got) != len(want) {
		t.Fatalf("Workloads() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Workloads() = %v, want %v", got, want)
		}
	}

	// WorkloadKinds collapses the same DisplayNames to distinct deployable kinds, sorted.
	gotK := r.WorkloadKinds()
	wantK := []string{"deployment", "job", "statefulset"}
	if len(gotK) != len(wantK) {
		t.Fatalf("WorkloadKinds() = %v, want %v", gotK, wantK)
	}
	for i := range wantK {
		if gotK[i] != wantK[i] {
			t.Fatalf("WorkloadKinds() = %v, want %v", gotK, wantK)
		}
	}

	// A bundle with no representative pod shows its workload count, not "pod pending".
	if got := r.RuntimeLine(); got != "4 workloads" {
		t.Errorf("bundle RuntimeLine = %q, want %q", got, "4 workloads")
	}

	// No k8sResourceInfo (compose/local/Tiltfile) -> no workloads or kinds.
	if wl := (&UIResource{}).Workloads(); wl != nil {
		t.Errorf("non-k8s Workloads() = %v, want nil", wl)
	}
	if k := (&UIResource{}).WorkloadKinds(); k != nil {
		t.Errorf("non-k8s WorkloadKinds() = %v, want nil", k)
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

func TestAllLines(t *testing.T) {
	v := loadView(t, "view_k8s.json")
	lines := v.LogList.AllLines()
	if len(lines) == 0 {
		t.Fatal("expected combined log lines")
	}

	var sawAPI, sawWorker bool
	for _, ln := range lines {
		if ln.Manifest == "api" && ln.Text == "listening on :8080" {
			sawAPI = true
		}
		if ln.Manifest == "worker" {
			sawWorker = true
			if ln.Level != "ERROR" {
				t.Errorf("worker line level = %q, want ERROR", ln.Level)
			}
		}
	}
	if !sawAPI {
		t.Error("combined lines missing api's 'listening on :8080' tagged to api")
	}
	if !sawWorker {
		t.Error("combined lines missing worker output")
	}
}
