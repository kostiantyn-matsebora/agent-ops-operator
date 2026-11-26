package system

import (
	"os"
	"path/filepath"
)

// SYSTEM_K3S_OOM_FIX=1 makes the node's containerd clamp a container's oom_score_adj to the value its own
// process holds. Kubelet gives the pause sandbox and Guaranteed pods a negative score, and lowering one
// needs CAP_SYS_RESOURCE, which a cloud sandbox withholds: runc then fails every pod sandbox with
// `oom_score_adj: Permission denied`. Clamped, nothing is lowered. Off on a workstation: a host that grants the
// capability should run the pod exactly as production does. On by default in a cloud session (oomFixEnabled).
const oomFixEnv = "SYSTEM_K3S_OOM_FIX"

// buildCAEnvName is buildCAEnv (buildca.go, which only builds with the system tag), so the discovery above can be
// unit tested without a cluster.
const buildCAEnvName = "SYSTEM_BUILD_CA"

const (
	oomDropInV3 = "[plugins.'io.containerd.cri.v1.runtime']\n  restrict_oom_score_adj = true\n"
	oomDropInV2 = "[plugins.\"io.containerd.grpc.v1.cri\"]\n  restrict_oom_score_adj = true\n"

	nodeContainerdDir = "/var/lib/rancher/k3s/agent/etc/containerd/"
)

// remoteSession is a cloud session, the one place the pack's sandbox defaults apply. The marker is the same one
// `.github/scripts/cloud-bootstrap.sh` tests.
func remoteSession() bool {
	switch os.Getenv("CLAUDE_CODE_REMOTE") {
	case "1", "true", "TRUE", "True":
		return true
	}
	return false
}

// oomFixEnabled: an explicit SYSTEM_K3S_OOM_FIX wins, either way. Unset, a cloud session turns it on, because its
// sandbox needs it and a session should not have to be told; a workstation leaves it off.
func oomFixEnabled() bool {
	if v, ok := os.LookupEnv(oomFixEnv); ok {
		return v == "1"
	}
	return remoteSession()
}

// cloudCABundles are where a cloud session keeps the host's FULL trust bundle, which re-signs HTTPS.
func cloudCABundles() []string {
	return []string{"/root/.ccr/ca-bundle.crt", os.Getenv("SSL_CERT_FILE")}
}

// buildCAPath is the bundle the image builds and the node must trust. An explicit SYSTEM_BUILD_CA wins, and an
// explicit empty one turns it off. Unset, a cloud session uses the first bundle it can read.
func buildCAPath() string {
	if v, ok := os.LookupEnv(buildCAEnvName); ok {
		return v
	}
	if !remoteSession() {
		return ""
	}
	for _, f := range cloudCABundles() {
		if f == "" {
			continue
		}
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			return f
		}
	}
	return ""
}

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
