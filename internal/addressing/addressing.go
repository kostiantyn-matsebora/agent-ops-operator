// Package addressing parses chat commands into agent addresses.
//
//	/<profile> <task>            -> profile's default agent
//	/<profile>:<agent> <task>    -> specific agent within the profile's repo
package addressing

import "regexp"

var cmdRe = regexp.MustCompile(`^/([\w-]+)(?::([\w-]+))?(?:@\S+)?\s*([\s\S]*)$`)

// Command is a parsed chat command.
type Command struct {
	Profile string
	Agent   string // optional override
	Rest    string
}

// Parse returns the parsed command, or ok=false for non-command text.
func Parse(text string) (Command, bool) {
	m := cmdRe.FindStringSubmatch(text)
	if m == nil {
		return Command{}, false
	}
	return Command{Profile: m[1], Agent: m[2], Rest: trimSpace(m[3])}, true
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
