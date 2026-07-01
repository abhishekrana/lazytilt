// Package tilt is a minimal client for a running Tilt instance's web API.
//
// It decodes the JSON "View" that the Tilt web UI consumes (GET /api/view) into
// a small set of hand-rolled structs. We deliberately do NOT import
// github.com/tilt-dev/tilt: its types drag in ~22 k8s.io modules, and we only
// need a read-only slice of the data plus a couple of CLI-driven actions.
//
// Tilt serializes the View as camelCase JSON; unknown fields are ignored.
package tilt

import "time"

// View is the top-level payload returned by GET /api/view.
type View struct {
	UISession     *UISession   `json:"uiSession"`
	UIResources   []UIResource `json:"uiResources"`
	LogList       LogList      `json:"logList"`
	TiltStartTime time.Time    `json:"tiltStartTime"`
	IsComplete    bool         `json:"isComplete"`
}

// Version returns the running Tilt version (e.g. "0.35.0"), or "" if unknown.
func (v *View) Version() string {
	if v.UISession == nil {
		return ""
	}
	return v.UISession.Status.RunningTiltBuild.Version
}

type UISession struct {
	Status UISessionStatus `json:"status"`
}

type UISessionStatus struct {
	RunningTiltBuild TiltBuild `json:"runningTiltBuild"`
}

type TiltBuild struct {
	Version string `json:"version"`
}

// UIResource is one resource (manifest) in the Tilt run.
type UIResource struct {
	Metadata ObjectMeta       `json:"metadata"`
	Status   UIResourceStatus `json:"status"`
}

// Name is the resource's display name, e.g. "web" or "(Tiltfile)".
func (r *UIResource) Name() string { return r.Metadata.Name }

type ObjectMeta struct {
	Name              string            `json:"name"`
	Labels            map[string]string `json:"labels"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp"`
}

type UIResourceStatus struct {
	UpdateStatus      string               `json:"updateStatus"`
	RuntimeStatus     string               `json:"runtimeStatus"`
	Order             int                  `json:"order"`
	BuildHistory      []BuildTerminated    `json:"buildHistory"`
	CurrentBuild      *BuildRunning        `json:"currentBuild"`
	PendingBuildSince *time.Time           `json:"pendingBuildSince"`
	HasPendingChanges bool                 `json:"hasPendingChanges"`
	Queued            bool                 `json:"queued"`
	DisableStatus     DisableStatus        `json:"disableStatus"`
	EndpointLinks     []Link               `json:"endpointLinks"`
	K8sResourceInfo   *K8sResourceInfo     `json:"k8sResourceInfo"`
	LocalResourceInfo *LocalResourceInfo   `json:"localResourceInfo"`
	Compose           *ComposeResourceInfo `json:"composeResourceInfo"`
	Waiting           *Waiting             `json:"waiting"`
}

type BuildTerminated struct {
	Error      string    `json:"error"`
	Warnings   []string  `json:"warnings"`
	StartTime  time.Time `json:"startTime"`
	FinishTime time.Time `json:"finishTime"`
	SpanID     string    `json:"spanID"`
}

type BuildRunning struct {
	StartTime time.Time `json:"startTime"`
	SpanID    string    `json:"spanID"`
}

// DisableStatus.State is one of "Enabled", "Disabled", "Error", or "" (pending).
type DisableStatus struct {
	State         string `json:"state"`
	EnabledCount  int    `json:"enabledCount"`
	DisabledCount int    `json:"disabledCount"`
}

type Link struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

type K8sResourceInfo struct {
	PodName            string `json:"podName"`
	PodStatus          string `json:"podStatus"`
	PodRestarts        int    `json:"podRestarts"`
	AllContainersReady bool   `json:"allContainersReady"`
	SpanID             string `json:"spanID"`
	// DisplayNames lists every k8s object the resource manages as "<name>:<kind>"
	// (e.g. "data-hub:deployment", "minio:service"). A helm_resource bundles a whole
	// release under one UIResource, so this is the only place the inner
	// deployments/statefulsets are exposed — k8sResourceInfo still carries just one
	// representative podName.
	DisplayNames []string `json:"displayNames"`
}

type LocalResourceInfo struct {
	PID    int64 `json:"pid"`
	IsTest bool  `json:"isTest"`
}

type ComposeResourceInfo struct {
	HealthStatus string `json:"healthStatus"`
}

type Waiting struct {
	Reason string `json:"reason"`
}

// LogList carries the resource logs. Segments are ordered; each segment's SpanID
// resolves to a manifest name via Spans[spanID].ManifestName (empty = global).
type LogList struct {
	Spans          map[string]LogSpan `json:"spans"`
	Segments       []LogSegment       `json:"segments"`
	FromCheckpoint int32              `json:"fromCheckpoint"`
	ToCheckpoint   int32              `json:"toCheckpoint"`
}

type LogSpan struct {
	ManifestName string `json:"manifestName"`
}

type LogSegment struct {
	SpanID string    `json:"spanId"`
	Time   time.Time `json:"time"`
	Text   string    `json:"text"`
	Level  string    `json:"level"`
}
