//go:build !linux && !darwin

package discovery

// scanProcesses is unsupported on this platform; lazytilt falls back to the
// -host/-port flags.
func scanProcesses() []Instance { return nil }
