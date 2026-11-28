package main

import (
	"log"
	"os"
)

// terminationLogPathEnv overrides where the termination message is written.
// Unset in production — the init container's terminationMessagePath is
// Kubernetes' own default, /dev/termination-log — and set only by this
// package's own tests, which have no such device to write to.
const terminationLogPathEnv = "WORKSPACE_SYNC_TERMINATION_LOG_PATH"

const defaultTerminationLogPath = "/dev/termination-log"

// maxTerminationMessage keeps the write well under the kubelet's own 4096
// byte cap, with headroom for whatever the kubelet adds around it.
const maxTerminationMessage = 3000

func terminationLogPath() string {
	if p := os.Getenv(terminationLogPathEnv); p != "" {
		return p
	}
	return defaultTerminationLogPath
}

// writeTerminationMessage records why this container failed, so the
// kubelet's reason reaches the Conversation's condition verbatim (design
// D9). Best-effort: a termination log this process cannot write must never
// mask the real failure's exit code by panicking or changing it.
func writeTerminationMessage(msg string) {
	if len(msg) > maxTerminationMessage {
		msg = msg[:maxTerminationMessage]
	}
	if err := os.WriteFile(terminationLogPath(), []byte(msg), 0o644); err != nil {
		log.Printf("workspace-sync: could not write termination message: %v", err)
	}
}
