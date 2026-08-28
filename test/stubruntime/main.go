// TEST STUB RUNTIME. NOT A RUNTIME. It runs no agent.
//
// A conforming client of the work contract — long-poll GET /work, report
// POST /work/done, exit 0 after the idle TTL — whose behaviour is SCRIPTED by
// the text of the input it is handed. It exists for the mechanisms an agent
// cannot be asked to exhibit on cue: a stale context handle, a crash that
// reports nothing, a stall past the idle TTL, a storage outage. Everything an
// agent CAN be asked to demonstrate is tested against the real runtime.
//
// It is built by the end-to-end pack only. .github/components.sh excludes test/,
// so no release publishes it; no chart default, sample or documented install
// references it; and it announces itself on every start and in every result.
//
// Directive vocabulary — the FIRST word of the task (USER_TASK, else the first
// line of the prompt that starts with one of these words):
//
//	echo            succeed, result = the input; mints/continues the context handle
//	fail            report a failed run
//	stale-context   succeed, returning a handle that names NOTHING
//	no-context      succeed, returning no handle at all
//	die             exit without reporting
//	stall           hold the unit past the idle TTL (never report)
//	storage-outage  report the context as unreachable — the breaker's input
//
// Anything else is `echo`. Identical input, identical report: no clock, no
// randomness and no pod name reaches a result.
//
// Context is a FILE per handle under $HOME/.agentops-stub/contexts/, which is
// what makes the handle mean something: a handle whose file is absent is a
// LOST context, and the run is failed rather than continued — exactly the
// contract's promised-and-lost rule.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const banner = "TEST STUB RUNTIME — not an agent runtime; its presence in a real deployment is a misconfiguration"

// resultPrefix marks every result so a stub answer can never be mistaken for
// an agent's in a transcript.
const resultPrefix = "[stub] "

type unit struct {
	RunID            string            `json:"runId"`
	Convo            string            `json:"convo"`
	RuntimeContextID string            `json:"runtimeContextId,omitempty"`
	ResumeSessionID  string            `json:"resumeSessionId,omitempty"`
	PromptText       string            `json:"promptText,omitempty"`
	PromptFile       string            `json:"promptFile,omitempty"`
	PromptVars       map[string]string `json:"promptVars,omitempty"`
}

type report struct {
	Convo            string `json:"convo"`
	RunID            string `json:"runId"`
	Status           string `json:"status"`
	ExitCode         *int32 `json:"exitCode,omitempty"`
	RuntimeContextID string `json:"runtimeContextId,omitempty"`
	Continuity       string `json:"continuity,omitempty"`
	ContinuityReason string `json:"continuityReason,omitempty"`
	Result           string `json:"result,omitempty"`
}

var directives = []string{"echo", "fail", "stale-context", "no-context", "die", "stall", "storage-outage"}

// input extracts the task text a unit carries: USER_TASK when the manager
// left rendering to the runtime, else the prompt itself.
func input(u unit) string {
	if t := strings.TrimSpace(u.PromptVars["USER_TASK"]); t != "" {
		return t
	}
	return strings.TrimSpace(u.PromptText)
}

// directive picks the behaviour: the first word of the input if it is one of
// the vocabulary, else the first LINE of the input that starts with one — a
// rendered prompt wraps the task in template prose — else echo.
func directive(text string) (string, string) {
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		word, rest, _ := strings.Cut(line, " ")
		for _, d := range directives {
			if word == d {
				if i == 0 {
					return d, strings.TrimSpace(rest)
				}
				return d, strings.TrimSpace(rest)
			}
		}
	}
	return "echo", text
}

// handleFor is the deterministic context handle for a conversation.
func handleFor(convo string) string { return "stub-ctx-" + convo }

func contextsDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/data/context"
	}
	return filepath.Join(home, ".agentops-stub", "contexts")
}

func contextExists(handle string) bool {
	_, err := os.Stat(filepath.Join(contextsDir(), handle))
	return err == nil
}

func touchContext(handle string) error {
	if err := os.MkdirAll(contextsDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(contextsDir(), handle), []byte(handle+"\n"), 0o644)
}

// perform decides the report for one unit. It returns (report, ok) where ok
// is false when the behaviour is to report NOTHING (die, stall); the caller
// acts on those. Pure apart from the context files, so identical input gives
// an identical report.
func perform(u unit) (report, bool) {
	text := input(u)
	d, rest := directive(text)
	handle := u.RuntimeContextID
	if handle == "" {
		handle = u.ResumeSessionID
	}
	r := report{Convo: u.Convo, RunID: u.RunID, Status: "succeeded"}
	// A handle we were asked to continue that names nothing is a LOST context:
	// promised-and-lost fails the run. The stale-context directive is how a
	// test plants such a handle; the failure is on the NEXT unit.
	if handle != "" && !contextExists(handle) && d != "storage-outage" {
		code := int32(1)
		r.Status, r.ExitCode = "failed", &code
		r.Continuity, r.ContinuityReason = "unavailable", "the handle "+handle+" names no context in "+contextsDir()
		r.Result = resultPrefix + "context " + handle + " is gone; refusing to continue without it"
		return r, true
	}
	switch d {
	case "fail":
		code := int32(1)
		r.Status, r.ExitCode = "failed", &code
		r.Result = resultPrefix + "failed on request"
		r.Continuity = continuity(handle)
		r.RuntimeContextID = keep(u.Convo, handle)
	case "stale-context":
		r.Result = resultPrefix + "returned a handle that names nothing"
		r.RuntimeContextID = "stub-stale-" + u.Convo
		r.Continuity = continuity(handle)
	case "no-context":
		r.Result = resultPrefix + "no handle reported"
		r.Continuity = "new"
	case "die":
		return report{}, false
	case "stall":
		return report{}, false
	case "storage-outage":
		code := int32(1)
		r.Status, r.ExitCode = "failed", &code
		r.Continuity, r.ContinuityReason = "unavailable", "context storage unreachable (stub storage-outage)"
		r.Result = resultPrefix + "context storage unreachable"
	default: // echo
		r.Result = resultPrefix + rest
		r.Continuity = continuity(handle)
		r.RuntimeContextID = keep(u.Convo, handle)
	}
	return r, true
}

func continuity(handle string) string {
	if handle == "" {
		return "new"
	}
	return "continued"
}

// keep continues the given handle or mints the conversation's own, writing
// the file that makes it real.
func keep(convo, handle string) string {
	if handle == "" {
		handle = handleFor(convo)
	}
	if err := touchContext(handle); err != nil {
		log.Printf("[stub] cannot write context %s: %v", handle, err)
	}
	return handle
}

func main() {
	control := strings.TrimSuffix(os.Getenv("CONTROL_URL"), "/")
	convo := os.Getenv("CONVO_ID")
	pod := os.Getenv("POD_NAME")
	ttlMin, _ := strconv.Atoi(os.Getenv("RUNTIME_IDLE_TTL_M"))
	if ttlMin <= 0 {
		ttlMin = 10
	}
	if control == "" || convo == "" {
		log.Fatal("[stub] CONTROL_URL and CONVO_ID are required")
	}
	log.Printf("[stub] %s (convo=%s pod=%s ttl=%dm)", banner, convo, pod, ttlMin)
	if err := run(control, convo, pod, time.Duration(ttlMin)*time.Minute, os.Exit); err != nil {
		log.Fatalf("[stub] %v", err)
	}
}

// run is the contract loop. exit is injected so a test can observe `die`.
func run(control, convo, pod string, ttl time.Duration, exit func(int)) error {
	client := &http.Client{Timeout: 40 * time.Second}
	idleSince := time.Now()
	for {
		if time.Since(idleSince) > ttl {
			log.Printf("[stub] idle for %s, exiting 0", ttl)
			exit(0)
			return nil
		}
		resp, err := client.Get(fmt.Sprintf("%s/work?convo=%s&pod=%s&wait=25", control, convo, pod))
		if err != nil {
			log.Printf("[stub] poll: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if resp.StatusCode == http.StatusNoContent {
			resp.Body.Close()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Printf("[stub] poll: %d %s", resp.StatusCode, body)
			time.Sleep(2 * time.Second)
			continue
		}
		var u unit
		if err := json.Unmarshal(body, &u); err != nil {
			log.Printf("[stub] unit: %v", err)
			continue
		}
		idleSince = time.Now()
		d, _ := directive(input(u))
		log.Printf("[stub] run %s directive=%s context=%q", u.RunID, d, u.RuntimeContextID)
		r, ok := perform(u)
		if !ok {
			switch d {
			case "die":
				log.Printf("[stub] dying without reporting, as scripted")
				exit(3)
				return nil
			case "stall":
				log.Printf("[stub] stalling past the idle TTL, as scripted")
				time.Sleep(ttl + time.Minute)
				exit(0)
				return nil
			}
		}
		if err := post(client, control+"/work/done", r); err != nil {
			return fmt.Errorf("report %s: %w", u.RunID, err)
		}
	}
}

func post(client *http.Client, url string, v any) error {
	b, _ := json.Marshal(v)
	for attempt := 0; attempt < 5; attempt++ {
		resp, err := client.Post(url, "application/json", bytes.NewReader(b))
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		log.Printf("[stub] report attempt %d: %v", attempt+1, err)
		time.Sleep(time.Second)
	}
	return fmt.Errorf("giving up")
}
