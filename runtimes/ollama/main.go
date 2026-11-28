// agentops ollama agent runtime — implements the AgentRuntime /work contract
// with the RUNTIME as the harness: the agent loop, tool dispatch, the
// transcript and the context handle are all this process's. Ollama is called
// only to produce the next message.
//
//  1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//  2. run the loop against the already-populated workspace, streaming to STDOUT
//  3. report     POST $CONTROL_URL/work/done
//  4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
//
// multi-repo-workspace moved cloning into the `workspace-sync` init
// container, ahead of this process ever starting — this runtime holds no
// git credential and runs no clone/fetch of its own. It reads WORKSPACE
// (always set) and START_DIR (set only when the conversation's workingDir
// resolves), and a work unit's AgentFile (an absolute path, exclusive with
// Agent) names the definition to read instead of resolving Agent under
// WORKSPACE itself.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const version = "0.1.0"

func env(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

func logf(format string, a ...any) { fmt.Fprintf(os.Stdout, format+"\n", a...) }

// runUnit builds this unit's own registry and runs it. The workspace itself
// is no longer this process's concern: workspace-sync populates it before
// this pod ever starts.
func runUnit(ctx context.Context, builtins Builtins, mcpc *MCPClient, agent *Agent, unit WorkUnit, logf func(string, ...any)) RunResult {
	// A registry PER UNIT: the built-ins, plus whatever the MCP servers list
	// now that the proxy has seen this unit's grants.
	agent.Registry = NewRegistry()
	builtins.Register(agent.Registry)
	mcpc.ListTools(ctx, agent.Registry, logf)
	return agent.Run(ctx, unit)
}

func main() {
	controlURL := os.Getenv("CONTROL_URL")
	convo := os.Getenv("CONVO_ID")
	ollamaURL := os.Getenv("OLLAMA_URL")
	model := os.Getenv("OLLAMA_MODEL")
	var missing []string
	for _, kv := range [][2]string{{"CONTROL_URL", controlURL}, {"CONVO_ID", convo}, {"OLLAMA_URL", ollamaURL}} {
		if kv[1] == "" {
			missing = append(missing, kv[0])
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "[runtime] required environment missing: %v\n", missing)
		os.Exit(1)
	}
	pod := os.Getenv("POD_NAME")
	workspace := env("WORKSPACE", "/workspace")
	// startDir is only ever set by the manager when the conversation's
	// workingDir resolves — absent means start in workspace itself.
	startDir := os.Getenv("START_DIR")
	home := env("HOME", "/data/context")
	ttl := time.Duration(envInt("RUNTIME_IDLE_TTL_M", 10)) * time.Minute
	numCtx := envInt("OLLAMA_NUM_CTX", 8192)
	chatTimeout := time.Duration(envInt("OLLAMA_TIMEOUT_S", 600)) * time.Second

	ctx := context.Background()
	ollama := &Ollama{URL: ollamaURL, Model: model, NumCtx: numCtx, NumPredict: envInt("OLLAMA_NUM_PREDICT", 0), KeepAlive: os.Getenv("OLLAMA_KEEP_ALIVE"), HTTP: newHTTPClient(chatTimeout)}

	startDisplay := startDir
	if startDisplay == "" {
		startDisplay = workspace
	}
	logf("[runtime] ollama runtime %s — convo=%s pod=%s ttl=%s workspace=%s start=%s model=%s endpoint=%s num_ctx=%d",
		version, convo, pod, ttl, workspace, startDisplay, model, ollamaURL, numCtx)
	info, err := ollama.Check(ctx)
	switch {
	case err != nil:
		logf("[runtime] startup check: endpoint=%s FAILED: %v — runs will fail until it answers", ollamaURL, err)
	case ollama.Model == "":
		logf("[runtime] startup check: endpoint=%s ok, NO MODEL CONFIGURED and the server lists %d — set OLLAMA_MODEL (ollama.model) to one of: %v", ollamaURL, len(info.Models), info.Models)
	case !info.Present:
		logf("[runtime] startup check: endpoint=%s ok, model=%s NOT PRESENT — pull it on the server; runs will fail naming it", ollamaURL, model)
	default:
		logf("[runtime] startup check: endpoint=%s ok, model=%s present, tools=%v%s", ollamaURL, ollama.Model, info.Tools,
			map[bool]string{true: " (the server's only model, chosen because OLLAMA_MODEL is unset)", false: ""}[model == ""])
	}

	builtins := Builtins{Workspace: workspace, StartDir: startDir, OutputMax: envInt("TOOL_OUTPUT_MAX", 64*1024),
		BashTimeout: time.Duration(envInt("BASH_TIMEOUT_S", 120)) * time.Second}
	cfg, err := readMCPConfig(env("MCP_CONFIG", "/etc/agentops/mcp.json"))
	if err != nil {
		logf("[mcp] config: %v — no MCP tools this pod", err)
	}
	mcpc := &MCPClient{CallTimeout: 2 * time.Minute}
	mcpc.Connect(ctx, cfg, logf)
	defer mcpc.Close()

	agent := &Agent{Chat: ollama, Workspace: workspace, NumCtx: numCtx, Model: ollama.Model,
		ModelCanCallTools: info.Tools || err != nil || !info.Present, // unknown is not "cannot"
		Store:             &ContextStore{Dir: filepath.Join(home, ".agentops", "contexts"), Sleep: time.Sleep},
		Out:               os.Stdout, Logf: logf}
	control := &Control{BaseURL: controlURL, Convo: convo, Pod: pod, HTTP: newHTTPClient(40 * time.Second)}

	lastWork := time.Now()
	for {
		if time.Since(lastWork) > ttl {
			logf("[runtime] idle TTL reached — exiting")
			return
		}
		unit, err := control.Poll(ctx)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		if unit == nil {
			continue
		}
		lastWork = time.Now()
		if info, err := ollama.Check(ctx); err == nil {
			agent.ModelCanCallTools = info.Tools || !info.Present
			agent.Model = ollama.Model
		}
		res := runUnit(ctx, builtins, mcpc, agent, *unit, logf)
		lastWork = time.Now()
		fmt.Fprintf(os.Stdout, "\n=== RESULT (%s) ===\n%s\n", res.Status, res.Result)
		if err := control.Report(ctx, unit.RunID, res, time.Sleep); err != nil {
			logf("[runtime] report: %v", err)
		}
	}
}
