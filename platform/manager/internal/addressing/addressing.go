// Package addressing parses chat commands into the Pipeline they address.
//
//	/<pipeline> <task>
//
// ONE SEGMENT, deliberately. There used to be a `/<pipeline>:<agent>` form that
// picked an agent definition inside the profile's repository, and it let
// whoever typed it choose an agent the WIRING never declared. A Pipeline names
// one profile and a profile names one agent, so the agent that runs is already
// fully determined — exactly as the toolsets and MCP servers are. Selecting
// your own is the same shape as a caller naming its own Pipeline over HTTP,
// which `pipeline-model` forbids in so many words.
//
// Consequence worth stating: text after the Pipeline name is simply the task.
// `/k8s-observe check:the pods` is a task containing a colon, which it always
// should have been.
package addressing

import "regexp"

var cmdRe = regexp.MustCompile(`^/([\w-]+)(?:@\S+)?\s*([\s\S]*)$`)

// Command is a parsed chat command.
type Command struct {
	Pipeline string
	Rest     string
}

// Parse returns the parsed command, or ok=false for non-command text.
func Parse(text string) (Command, bool) {
	m := cmdRe.FindStringSubmatch(text)
	if m == nil {
		return Command{}, false
	}
	return Command{Pipeline: m[1], Rest: trimSpace(m[2])}, true
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
