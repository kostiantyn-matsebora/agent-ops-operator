## Containers (`docker` OR `nerdctl`)

**CHECK FOR BOTH, NEVER FOR `docker` ALONE.** Either one runs the system pack.

| Fact | Consequence |
|---|---|
| Rancher Desktop ships both under `~/.rd/bin` | look there as well as on `PATH` |
| `nerdctl` answers only while the application is running | a stopped application reads as "no engine" |
| The pack's `lookPath("docker")` falls back to `nerdctl` | `execpath.go` owns the fallback |
| k3d needs a real Docker API | use Rancher Desktop's `moby` engine, `DOCKER_HOST=unix://$HOME/.rd/docker.sock` |
| The `containerd` engine's socket resets every connection | k3d cannot use it |
