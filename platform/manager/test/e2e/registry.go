//go:build e2e

package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// A LOCAL AUTHENTICATED REGISTRY, for the one test that exercises the pull
// path. Every other image is IMPORTED into the node and is therefore never
// pulled — so without this, "images pull, including through imagePullSecrets"
// would be vacuously true.
//
// The registry is a `registry:2` container on the cluster's docker network,
// reachable from the node by its container name over plain HTTP (the cluster
// is created with a registries.yaml naming it as such) and from the runner
// through a published port. Its credential is a THROWAWAY htpasswd minted per
// run — never the GHCR token, which has no business in a test.

const registryName = "agentops-e2e-registry"
const registryHost = registryName + ":5000"

// registriesYAML is handed to k3d at cluster creation so containerd treats the
// registry as plain HTTP. Naming a container that does not exist yet is fine:
// the mirror is consulted only when something pulls from it.
const registriesYAML = `mirrors:
  "` + registryHost + `":
    endpoint:
      - "http://` + registryHost + `"
`

// Registry is the running container and its throwaway credential.
type Registry struct {
	User, Password string
	HostPort       string // 127.0.0.1:<port>, for docker push from the runner
}

// StartRegistry runs the registry on the cluster's network, or adopts a
// running one. The htpasswd is written to a temp dir the container mounts —
// which is fine here because the runner's daemon is local; a VM-backed daemon
// mounting /tmp is the one case build-test.md warns about, and it is why the
// file goes under the working directory instead.
func StartRegistry(ctx context.Context, c *Cluster) (*Registry, error) {
	r := &Registry{User: "e2e", Password: fmt.Sprintf("pw-%d", os.Getpid())}
	if out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", registryName).Output(); err == nil && strings.TrimSpace(string(out)) == "true" {
		_ = exec.Command("docker", "rm", "-f", registryName).Run()
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(repoRoot(), "platform", "manager", "test", "e2e", ".registry")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "htpasswd"), []byte(r.User+":"+string(hash)+"\n"), 0o644); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "docker", "run", "-d", "--name", registryName,
		"--network", "k3d-"+c.Name,
		"-p", "127.0.0.1:0:5000",
		"-v", dir+":/auth:ro",
		"-e", "REGISTRY_AUTH=htpasswd",
		"-e", "REGISTRY_AUTH_HTPASSWD_REALM=e2e",
		"-e", "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd",
		"registry:2")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("starting the registry: %v\n%s", err, out)
	}
	out, err := exec.Command("docker", "port", registryName, "5000/tcp").Output()
	if err != nil {
		return nil, err
	}
	// "127.0.0.1:32768" (possibly one line per family)
	r.HostPort = strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return r, nil
}

// Stop removes the container.
func (r *Registry) Stop() { _ = exec.Command("docker", "rm", "-f", registryName).Run() }

// Push tags a local image into the registry and pushes it with the throwaway
// credential — through a private docker config dir, so no credential helper
// (and no gpg agent) is consulted and nothing is written to the user's
// docker config.
func (r *Registry) Push(ctx context.Context, localTag, name string) (string, error) {
	cfgDir, err := os.MkdirTemp("", "agentops-e2e-docker")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(cfgDir)
	auth := base64.StdEncoding.EncodeToString([]byte(r.User + ":" + r.Password))
	cfg, _ := json.Marshal(map[string]any{"auths": map[string]any{r.HostPort: map[string]string{"auth": auth}}})
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), cfg, 0o600); err != nil {
		return "", err
	}
	hostRef := r.HostPort + "/" + name
	if out, err := exec.CommandContext(ctx, "docker", "tag", localTag, hostRef).CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker tag: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "docker", "--config", cfgDir, "push", hostRef).CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker push: %v\n%s", err, out)
	}
	// The node reaches it by the container name, not the published port.
	return registryHost + "/" + name, nil
}

// DockerConfigJSON is the imagePullSecret payload for the in-cluster name.
func (r *Registry) DockerConfigJSON() []byte {
	auth := base64.StdEncoding.EncodeToString([]byte(r.User + ":" + r.Password))
	b, _ := json.Marshal(map[string]any{"auths": map[string]any{registryHost: map[string]string{
		"username": r.User, "password": r.Password, "auth": auth}}})
	return b
}
