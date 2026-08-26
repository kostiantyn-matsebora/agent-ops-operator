// agentops ollama agent runtime — implements the AgentRuntime /work contract
// with the RUNTIME as the harness: the agent loop, tool dispatch, the
// transcript and the context handle are all this process's. Ollama is called
// only to produce the next message.
//
//  1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//  2. run the loop against the profile's checkout, streaming to STDOUT
//  3. report     POST $CONTROL_URL/work/done
//  4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
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
	workspace := env("WORKSPACE", "/data/workspace")
	home := env("HOME", "/data/context")
	ttl := time.Duration(envInt("RUNTIME_IDLE_TTL_M", 10)) * time.Minute
	numCtx := envInt("OLLAMA_NUM_CTX", 8192)
	chatTimeout := time.Duration(envInt("OLLAMA_TIMEOUT_S", 600)) * time.Second

	ctx := context.Background()
	ollama := &Ollama{URL: ollamaURL, Model: model, NumCtx: numCtx, KeepAlive: os.Getenv("OLLAMA_KEEP_ALIVE"), HTTP: newHTTPClient(chatTimeout)}

	logf("[runtime] ollama runtime %s — convo=%s pod=%s ttl=%s workspace=%s model=%s endpoint=%s num_ctx=%d",
		version, convo, pod, ttl, workspace, model, ollamaURL, numCtx)
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

	repo := Repo{URL: os.Getenv("REPO_URL"), Ref: env("REPO_REF", "master"), AuthType: os.Getenv("GIT_AUTH_TYPE"),
		SSHKey: os.Getenv("GIT_SSH_KEY"), Token: os.Getenv("GIT_TOKEN"), Workspace: workspace}
	if err := repo.Sync(ctx); err != nil {
		logf("[runtime] initial sync: %v", err)
	}

	builtins := Builtins{Workspace: workspace, OutputMax: envInt("TOOL_OUTPUT_MAX", 64*1024),
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
		if err := repo.Sync(ctx); err != nil {
			logf("[runtime] sync: %v", err)
		}
		if info, err := ollama.Check(ctx); err == nil {
			agent.ModelCanCallTools = info.Tools || !info.Present
			agent.Model = ollama.Model
		}
		// A registry PER UNIT: the built-ins, plus whatever the MCP servers list
		// now that the proxy has seen this unit's grants.
		agent.Registry = NewRegistry()
		builtins.Register(agent.Registry)
		mcpc.ListTools(ctx, agent.Registry, logf)
		res := agent.Run(ctx, *unit)
		lastWork = time.Now()
		fmt.Fprintf(os.Stdout, "\n=== RESULT (%s) ===\n%s\n", res.Status, res.Result)
		if err := control.Report(ctx, unit.RunID, res, time.Sleep); err != nil {
			logf("[runtime] report: %v", err)
		}
	}
}
