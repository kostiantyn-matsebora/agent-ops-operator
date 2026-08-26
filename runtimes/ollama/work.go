// Work contract plumbing: the unit the manager dispatches, the report this
// runtime returns, the long-poll and the retried report.
//
// The types MIRROR platform/manager/internal/dispatch.WorkUnit and
// internal/httpapi.workDone field for field. Nothing here imports the manager —
// this module is self-contained by the repository's rule, and the wire format
// is the contract, not the Go type.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// WorkUnit is one dispatched run.
type WorkUnit struct {
	RunID    string  `json:"runId"`
	Convo    string  `json:"convo"`
	ThreadID *string `json:"threadId,omitempty"`
	// RuntimeContextID is THIS runtime's own handle, echoed back from the last
	// run. Empty means start fresh.
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	// ResumeSessionID is the retired spelling, read for one release.
	ResumeSessionID string            `json:"resumeSessionId,omitempty"`
	PromptFile      string            `json:"promptFile,omitempty"`
	PromptText      string            `json:"promptText,omitempty"`
	PromptVars      map[string]string `json:"promptVars,omitempty"`
	Agent           string            `json:"agent,omitempty"`
	SystemPrompt    string            `json:"systemPrompt,omitempty"`
	AllowedTools    string            `json:"allowedTools,omitempty"`
	ToolsMode       string            `json:"toolsMode,omitempty"`
	MaxTurns        int32             `json:"maxTurns,omitempty"`
}

// ContextID reads the handle under its current name, falling back to the
// retired one for one release.
func (u WorkUnit) ContextID() string {
	if u.RuntimeContextID != "" {
		return u.RuntimeContextID
	}
	return u.ResumeSessionID
}

// Continuity values the manager understands.
const (
	ContinuityContinued   = "continued"
	ContinuityNew         = "new"
	ContinuityUnavailable = "unavailable"
)

// RunResult is what executing a unit produced, before it is addressed.
type RunResult struct {
	Status           string `json:"status"`
	ExitCode         int32  `json:"exitCode"`
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	Continuity       string `json:"continuity,omitempty"`
	ContinuityReason string `json:"continuityReason,omitempty"`
	Result           string `json:"result,omitempty"`
}

// doneReport is the POST /work/done body.
type doneReport struct {
	Convo string `json:"convo"`
	RunID string `json:"runId"`
	RunResult
	// SessionID carries the handle under the retired spelling for one release,
	// so this image also works against an older manager.
	SessionID string `json:"sessionId,omitempty"`
}

// Control talks to $CONTROL_URL.
type Control struct {
	BaseURL string
	Convo   string
	Pod     string
	HTTP    *http.Client
}

// Poll long-polls /work once. A nil unit with a nil error means no work.
func (c *Control) Poll(ctx context.Context) (*WorkUnit, error) {
	q := url.Values{"convo": {c.Convo}, "pod": {c.Pod}, "wait": {"25"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/work?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("GET /work: %s", resp.Status)
	}
	var unit WorkUnit
	if err := json.NewDecoder(resp.Body).Decode(&unit); err != nil {
		return nil, fmt.Errorf("GET /work: decode: %w", err)
	}
	return &unit, nil
}

// Report POSTs /work/done with the reference runtime's cadence: up to 60
// attempts, ten seconds apart. The report is the one durable fact about the
// run, so it is worth ten minutes of trying.
func (c *Control) Report(ctx context.Context, runID string, r RunResult, sleep func(time.Duration)) error {
	body, _ := json.Marshal(doneReport{Convo: c.Convo, RunID: runID, RunResult: r, SessionID: r.RuntimeContextID})
	var last error
	for i := 0; i < 60; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/work/done", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.HTTP.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return nil
			}
			last = fmt.Errorf("POST /work/done: %s", resp.Status)
		} else {
			last = err
		}
		sleep(10 * time.Second)
	}
	return last
}
