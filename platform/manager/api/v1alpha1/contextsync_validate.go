package v1alpha1

import (
	"fmt"
	"path"
	"strings"
)

// Validate checks a ContextSync declaration.
//
// WHERE THIS IS ENFORCED, and why it is not a readiness condition: nothing
// writes AgentRuntimeStatus. The field exists, but no reconciler owns this CR,
// so a "report it on Ready" rule would mean inventing a controller whose only
// job is to hold one condition. Validation therefore lands in two places that
// already exist:
//
//   - The CRD SCHEMA rejects the structural errors at admission — an empty
//     paths list cannot be written at all, which is stronger than reporting it
//     afterwards.
//   - This function runs when a runtime is RESOLVED to build a pod, and its
//     error surfaces on the Conversation's RuntimeStarted condition, where an
//     operator is already looking when nothing is running.
//
// A nil ContextSync is valid: absent means today's behaviour.
func (c *ContextSync) Validate() error {
	if c == nil {
		return nil
	}
	if len(c.Paths) == 0 {
		return fmt.Errorf("contextSync.paths must name at least one path; " +
			"an empty include list would persist nothing while appearing configured")
	}
	for _, p := range c.Paths {
		if err := validateContextPath("paths", p); err != nil {
			return err
		}
	}
	for _, p := range c.Exclude {
		if err := validateContextPath("exclude", p); err != nil {
			return err
		}
	}
	if c.Interval != nil && c.Interval.Duration < 0 {
		return fmt.Errorf("contextSync.interval must not be negative (0 means work-boundary checkpoints only)")
	}
	if c.Retain < 0 {
		return fmt.Errorf("contextSync.retain must not be negative")
	}
	return nil
}

// validateContextPath rejects a glob that reaches outside the runtime's home.
//
// The paths are RELATIVE TO HOME and are copied by a process that mounts the
// durable volume. A path escaping home would have that process copy files the
// agent's own container is deliberately not given — which is the isolation this
// whole arrangement buys. Neither absolute paths nor `..` are ever a legitimate
// way to name an agent's own context.
func validateContextPath(field, p string) error {
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("contextSync.%s contains an empty path", field)
	}
	if path.IsAbs(p) {
		return fmt.Errorf("contextSync.%s %q must be relative to the runtime's home, not absolute", field, p)
	}
	if p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") {
		return fmt.Errorf("contextSync.%s %q must not escape the runtime's home", field, p)
	}
	return nil
}
