package system

import (
	"os"
	"path/filepath"
)

// SYSTEM_K3S_OOM_FIX=1 makes the node's containerd clamp a container's oom_score_adj to the value its own
// process holds. Kubelet gives the pause sandbox and Guaranteed pods a negative score, and lowering one
// needs CAP_SYS_RESOURCE, which a cloud sandbox withholds: runc then fails every pod sandbox with
// `oom_score_adj: Permission denied`. Clamped, nothing is lowered. Off by default: a host that grants the
// capability should run the pod exactly as production does.
const oomFixEnv = "SYSTEM_K3S_OOM_FIX"

const (
	oomDropInV3 = "[plugins.'io.containerd.cri.v1.runtime']\n  restrict_oom_score_adj = true\n"
	oomDropInV2 = "[plugins.\"io.containerd.grpc.v1.cri\"]\n  restrict_oom_score_adj = true\n"

	nodeContainerdDir = "/var/lib/rancher/k3s/agent/etc/containerd/"
)

func oomFixEnabled() bool { return os.Getenv(oomFixEnv) == "1" }

// oomFixArgs writes the two drop-ins (containerd 2's config format, and 1's for an older k3s image) into two
// temporary DIRECTORIES and mounts each where k3s reads containerd drop-ins. A directory, never a single file:
// `k3d kubeconfig get` copies out of the node with `docker cp`, which tries to create a file mount's path as a
// directory and fails with `file exists`, so a run against a live cluster (SYSTEM_REUSE=1) could not start.
// The returned cleanup removes the directories once `k3d cluster create` has read them.
func oomFixArgs() (args []string, cleanup func(), err error) {
	cleanup = func() {}
	if !oomFixEnabled() {
		return nil, cleanup, nil
	}
	var dirs []string
	cleanup = func() {
		for _, d := range dirs {
			_ = os.RemoveAll(d)
		}
	}
	for dir, body := range map[string]string{"config-v3.toml.d": oomDropInV3, "config.toml.d": oomDropInV2} {
		d, derr := os.MkdirTemp("", "agentops-e2e-oom-*")
		if derr != nil {
			cleanup()
			return nil, func() {}, derr
		}
		dirs = append(dirs, d)
		if werr := os.WriteFile(filepath.Join(d, "10-restrict-oom.toml"), []byte(body), 0o644); werr != nil {
			cleanup()
			return nil, func() {}, werr
		}
		args = append(args, "--volume", d+":"+nodeContainerdDir+dir+"@server:*")
	}
	return args, cleanup, nil
}
