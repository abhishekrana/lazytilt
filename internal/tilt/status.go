package tilt

import (
	"fmt"
	"sort"
	"strings"
)

// Status is the single, backend-agnostic state we render for a resource. It
// collapses Tilt's separate updateStatus (build) and runtimeStatus (pod/
// container) into one value so k8s, docker-compose and local resources all map
// onto the same small set of colors/glyphs.
type Status int

const (
	StatusNone     Status = iota // idle / not built or run yet
	StatusOK                     // built & running fine
	StatusPending                // queued / waiting to build or start
	StatusBuilding               // build in progress
	StatusError                  // build or runtime error
	StatusDisabled               // resource is disabled
)

// Label is a short lowercase word for the header status field.
func (s Status) Label() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusPending:
		return "pending"
	case StatusBuilding:
		return "building"
	case StatusError:
		return "error"
	case StatusDisabled:
		return "disabled"
	default:
		return "idle"
	}
}

// State derives the combined status for a resource.
func (r *UIResource) State() Status {
	if r.Status.DisableStatus.State == "Disabled" {
		return StatusDisabled
	}
	if r.Status.UpdateStatus == "error" || r.Status.RuntimeStatus == "error" {
		return StatusError
	}
	if r.Status.UpdateStatus == "in_progress" || r.Status.CurrentBuild != nil {
		return StatusBuilding
	}
	if r.Status.UpdateStatus == "pending" || r.Status.RuntimeStatus == "pending" ||
		r.Status.PendingBuildSince != nil || r.Status.Queued {
		return StatusPending
	}
	if r.Status.UpdateStatus == "ok" {
		return StatusOK
	}
	return StatusNone
}

// IsDisabled reports whether the resource is currently disabled.
func (r *UIResource) IsDisabled() bool {
	return r.Status.DisableStatus.State == "Disabled"
}

// Backend reports the resource backend: "k8s", "compose", "local", or "" for
// pseudo-resources like (Tiltfile).
func (r *UIResource) Backend() string {
	switch {
	case r.Status.K8sResourceInfo != nil:
		return "k8s"
	case r.Status.Compose != nil:
		return "compose"
	case r.Status.LocalResourceInfo != nil && r.Status.LocalResourceInfo.PID != 0:
		return "local"
	default:
		return ""
	}
}

// workloadKinds are the object kinds (as they appear in DisplayNames) we treat
// as deployable workloads — the "deployments/pods" the resource runs. Everything
// else (service, configmap, secret, pvc, serviceaccount) is supporting glue we
// don't surface.
var workloadKinds = map[string]bool{
	"deployment": true, "statefulset": true, "daemonset": true,
	"replicaset": true, "job": true, "cronjob": true, "rollout": true,
}

// Workloads returns the workload names this k8s resource manages, sorted. For a
// helm_resource (a whole release under one Tilt resource) this is the list of
// inner deployments/statefulsets — e.g. ["auth-service", "data-hub", ...]. Empty
// for non-k8s resources or when Tilt reports no object list.
func (r *UIResource) Workloads() []string {
	if r.Status.K8sResourceInfo == nil {
		return nil
	}
	var out []string
	for _, dn := range r.Status.K8sResourceInfo.DisplayNames {
		name, kind, ok := strings.Cut(dn, ":")
		if ok && workloadKinds[kind] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// WorkloadKinds returns the distinct workload kinds this k8s resource manages,
// sorted — e.g. ["deployment"] for a normal resource, or ["deployment","job",
// "statefulset"] for a helm release. Empty for non-k8s resources or ones with no
// workload objects. Lets the detail view name the resource type (deployment vs job).
func (r *UIResource) WorkloadKinds() []string {
	if r.Status.K8sResourceInfo == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, dn := range r.Status.K8sResourceInfo.DisplayNames {
		if _, kind, ok := strings.Cut(dn, ":"); ok && workloadKinds[kind] && !seen[kind] {
			seen[kind] = true
			out = append(out, kind)
		}
	}
	sort.Strings(out)
	return out
}

// RuntimeLine is the single adaptive line shown in the detail header. It keeps
// the same position and styling across backends; only the text differs.
func (r *UIResource) RuntimeLine() string {
	switch r.Backend() {
	case "k8s":
		k := r.Status.K8sResourceInfo
		if k.PodName == "" {
			// A helm_resource bundles many workloads under one UIResource and reports no
			// single representative pod, so "pod pending" would lie — the release's pods
			// may be running fine. Show the workload count instead. A plain resource with
			// no pod yet is genuinely still pending.
			if n := len(r.Workloads()); n > 1 {
				return fmt.Sprintf("%d workloads", n)
			}
			return "pod pending"
		}
		line := fmt.Sprintf("pod %s · %s", k.PodName, k.PodStatus)
		if k.PodRestarts > 0 {
			line += fmt.Sprintf(" · restarts %d", k.PodRestarts)
		}
		return line
	case "compose":
		if h := r.Status.Compose.HealthStatus; h != "" {
			return "container · " + h
		}
		return "container"
	case "local":
		return fmt.Sprintf("pid %d", r.Status.LocalResourceInfo.PID)
	default:
		return ""
	}
}

// LastError returns the most recent build error for the resource, if any.
func (r *UIResource) LastError() string {
	if len(r.Status.BuildHistory) > 0 {
		return r.Status.BuildHistory[0].Error
	}
	return ""
}

// LastWarnings returns the most recent build's warnings, if any. Tilt keeps them
// out of the resource's error state, so a healthy resource can still carry them.
func (r *UIResource) LastWarnings() []string {
	if len(r.Status.BuildHistory) > 0 {
		return r.Status.BuildHistory[0].Warnings
	}
	return nil
}

// LastBuild returns the most recent build with valid timestamps (start and finish
// present and ordered), and whether one was found. The detail view uses it for
// both the build duration and how long ago it finished.
func (r *UIResource) LastBuild() (BuildTerminated, bool) {
	for i := range r.Status.BuildHistory {
		b := r.Status.BuildHistory[i]
		if !b.StartTime.IsZero() && !b.FinishTime.IsZero() && !b.FinishTime.Before(b.StartTime) {
			return b, true
		}
	}
	return BuildTerminated{}, false
}

// Endpoints returns the resource's endpoint links (e.g. forwarded ports).
func (r *UIResource) Endpoints() []Link { return r.Status.EndpointLinks }

// LabelNames returns the resource's label keys, sorted, for display.
func (r *UIResource) LabelNames() []string {
	if len(r.Metadata.Labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(r.Metadata.Labels))
	for k := range r.Metadata.Labels {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Resources returns the resources sorted by Tilt's own order, matching the web UI.
func (v *View) Resources() []UIResource {
	out := make([]UIResource, len(v.UIResources))
	copy(out, v.UIResources)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Status.Order < out[j].Status.Order
	})
	return out
}

// Counts returns the number of errored and ok-or-better resources, plus the
// total of non-disabled resources — used for the "✕N ✓ok/total" summary.
func (v *View) Counts() (errs, ok, total int) {
	for i := range v.UIResources {
		r := &v.UIResources[i]
		if r.IsDisabled() {
			continue
		}
		total++
		switch r.State() {
		case StatusError:
			errs++
		case StatusOK:
			ok++
		}
	}
	return errs, ok, total
}

// StatusCounts tallies a view's non-disabled resources by State. It backs the
// cross-instance overview and the top-bar health badges.
type StatusCounts struct {
	Error, Building, Pending, OK, Idle, Total int
}

// StatusCounts returns the per-state tally for this view.
func (v *View) StatusCounts() StatusCounts {
	var c StatusCounts
	for i := range v.UIResources {
		r := &v.UIResources[i]
		if r.IsDisabled() {
			continue
		}
		c.Total++
		switch r.State() {
		case StatusError:
			c.Error++
		case StatusBuilding:
			c.Building++
		case StatusPending:
			c.Pending++
		case StatusOK:
			c.OK++
		default:
			c.Idle++
		}
	}
	return c
}
