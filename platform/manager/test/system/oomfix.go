package system

import (
	"os"
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

// oomFixArgs writes the two drop-ins (containerd 2's config format, and 1's for an older k3s image) to
// temporary files and mounts them where k3s reads containerd drop-ins. The returned cleanup removes the files
// once `k3d cluster create` has read them.
func oomFixArgs() (args []string, cleanup func(), err error) {
	cleanup = func() {}
	if !oomFixEnabled() {
		return nil, cleanup, nil
	}
	var files []string
	cleanup = func() {
		for _, f := range files {
			_ = os.Remove(f)
		}
	}
	for dir, body := range map[string]string{"config-v3.toml.d": oomDropInV3, "config.toml.d": oomDropInV2} {
		f, cerr := os.CreateTemp("", "agentops-e2e-oom-*.toml")
		if cerr != nil {
			cleanup()
			return nil, func() {}, cerr
		}
		files = append(files, f.Name())
		_, werr := f.WriteString(body)
		clerr := f.Close()
		if werr != nil || clerr != nil {
			cleanup()
			if werr != nil {
				return nil, func() {}, werr
			}
			return nil, func() {}, clerr
		}
		args = append(args, "--volume", f.Name()+":"+nodeContainerdDir+dir+"/10-restrict-oom.toml@server:*")
	}
	return args, cleanup, nil
}
