package tilt

import (
	"fmt"
	"sort"
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

// RuntimeLine is the single adaptive line shown in the detail header. It keeps
// the same position and styling across backends; only the text differs.
func (r *UIResource) RuntimeLine() string {
	switch r.Backend() {
	case "k8s":
		k := r.Status.K8sResourceInfo
		if k.PodName == "" {
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
