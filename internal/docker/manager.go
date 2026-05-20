package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/build"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	volume "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
)

const (
	NetworkName       = "akama-net"
	PostgresContainer = "akama-postgres"
	OllamaContainer   = "akama-ollama"
	DaemonContainer   = "akama-daemon"
	DaemonImage       = "akama-daemon:latest"
	PostgresImage     = "docker.io/pgvector/pgvector:pg16"
	OllamaImage       = "docker.io/ollama/ollama:latest"
	WorkspacesVolume  = "akama-workspaces"
	// PostgresURL is the connection string for the host-side CLI.
	PostgresURL  = "postgres://akama:akama@127.0.0.1:5432/akama"
	PostgresPort = "5432"
	// OllamaURL is the Ollama API endpoint for the host-side CLI.
	OllamaURL = "http://localhost:11434"
	// InternalPostgresURL is the connection string used by the daemon container
	// to reach postgres via the akama-net bridge network.
	InternalPostgresURL = "postgres://akama:akama@akama-postgres:5432/akama"
	// InternalOllamaURL is the Ollama endpoint used by the daemon container.
	InternalOllamaURL = "http://akama-ollama:11434"
)

func NewClient() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return cli, nil
}

func EnsureNetwork(ctx context.Context, cli *client.Client) error {
	networks, err := cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", NetworkName)),
	})
	if err != nil {
		return fmt.Errorf("list networks: %w", err)
	}
	for _, n := range networks {
		if n.Name == NetworkName {
			return nil
		}
	}
	_, err = cli.NetworkCreate(ctx, NetworkName, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{"app": "akama"},
	})
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}
	return nil
}

func ContainerExists(ctx context.Context, cli *client.Client, name string) (bool, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return false, err
	}
	return len(containers) > 0, nil
}

func ContainerRunning(ctx context.Context, cli *client.Client, name string) (bool, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if n == "/"+name {
				return c.State == "running", nil
			}
		}
	}
	return false, nil
}

// ContainerHealthy returns true only when the named container is running and
// has not been restarted (restart count == 0 means it came up clean). A
// container that has restarted is considered unhealthy and should be replaced.
func ContainerHealthy(ctx context.Context, cli *client.Client, name string) (bool, error) {
	inspect, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, nil
	}
	if inspect.State == nil || !inspect.State.Running {
		return false, nil
	}
	return inspect.RestartCount == 0, nil
}

func PullImage(ctx context.Context, cli *client.Client, imageRef string, out io.Writer) error {
	reader, err := cli.ImagePull(ctx, imageRef, dockerimage.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", imageRef, err)
	}
	defer reader.Close()
	if out != nil {
		fd, isTerminal := termFD(out)
		return jsonmessage.DisplayJSONMessagesStream(reader, out, fd, isTerminal, nil)
	}
	io.Copy(io.Discard, reader)
	return nil
}

// termFD returns the file descriptor and TTY status of w if it is an *os.File.
func termFD(w io.Writer) (uintptr, bool) {
	f, ok := w.(*os.File)
	if !ok {
		return 0, false
	}
	fd := f.Fd()
	return fd, isTerminal(fd)
}

func isTerminal(fd uintptr) bool {
	_, _, err := term.GetSize(int(fd))
	return err == nil
}

func EnsurePostgresContainer(ctx context.Context, cli *client.Client, hostPort string) error {
	running, err := ContainerRunning(ctx, cli, PostgresContainer)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	exists, err := ContainerExists(ctx, cli, PostgresContainer)
	if err != nil {
		return err
	}
	if exists {
		return cli.ContainerStart(ctx, PostgresContainer, container.StartOptions{})
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: PostgresImage,
		Env: []string{
			"POSTGRES_USER=akama",
			"POSTGRES_PASSWORD=akama",
			"POSTGRES_DB=akama",
		},
		Labels: map[string]string{"app": "akama"},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"5432/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: hostPort}},
		},
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
	}, nil, nil, PostgresContainer)
	if err != nil {
		return fmt.Errorf("create postgres container: %w", err)
	}

	if err := cli.NetworkConnect(ctx, NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect postgres to network: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start postgres: %w", err)
	}

	return nil
}

func EnsureOllamaContainer(ctx context.Context, cli *client.Client) error {
	running, err := ContainerRunning(ctx, cli, OllamaContainer)
	if err != nil {
		return err
	}
	if running {
		return nil
	}

	exists, err := ContainerExists(ctx, cli, OllamaContainer)
	if err != nil {
		return err
	}
	if exists {
		return cli.ContainerStart(ctx, OllamaContainer, container.StartOptions{})
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:  OllamaImage,
		Labels: map[string]string{"app": "akama"},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"11434/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "11434"}},
		},
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		Resources: container.Resources{
			DeviceRequests: []container.DeviceRequest{
				{Driver: "nvidia", Count: -1, Capabilities: [][]string{{"gpu"}}},
			},
		},
	}, nil, nil, OllamaContainer)
	if err != nil {
		return fmt.Errorf("create ollama container: %w", err)
	}

	if err := cli.NetworkConnect(ctx, NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect ollama to network: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start ollama: %w", err)
	}

	return nil
}


func StartContainer(ctx context.Context, cli *client.Client, name string) error {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if n == "/"+name {
				if c.State == "running" {
					return nil
				}
				return cli.ContainerStart(ctx, c.ID, container.StartOptions{})
			}
		}
	}
	return fmt.Errorf("container %s not found", name)
}

func StopContainer(ctx context.Context, cli *client.Client, name string) error {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if n == "/"+name {
				timeout := 30
				return cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout})
			}
		}
	}
	return nil
}

func RemoveContainer(ctx context.Context, cli *client.Client, name string) error {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if n == "/"+name {
				return cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
			}
		}
	}
	return nil
}

func EnsureVolume(ctx context.Context, cli *client.Client, name string) error {
	_, err := cli.VolumeInspect(ctx, name)
	if err == nil {
		return nil
	}
	_, err = cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: map[string]string{"app": "akama"},
	})
	return err
}

func RemoveVolume(ctx context.Context, cli *client.Client, volumeName string) error {
	return cli.VolumeRemove(ctx, volumeName, true)
}

func WaitHealthy(ctx context.Context, cli *client.Client, containerName string, checkFn func(context.Context) error) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := checkFn(ctx); err == nil {
				return nil
			}
		}
	}
}

// GetContainerHostPort returns the host port bound to the given container port (e.g. "5432/tcp").
// Returns "" if the container is not running or has no binding for that port.
func GetContainerHostPort(ctx context.Context, cli *client.Client, name, containerPort string) string {
	info, err := cli.ContainerInspect(ctx, name)
	if err != nil {
		return ""
	}
	bindings := info.NetworkSettings.Ports[nat.Port(containerPort)]
	if len(bindings) == 0 {
		return ""
	}
	return bindings[0].HostPort
}

func PullAndEnsureModel(ctx context.Context, cli *client.Client, model string) error {
	execResp, err := cli.ContainerExecCreate(ctx, OllamaContainer, container.ExecOptions{
		Cmd: []string{"ollama", "pull", model},
	})
	if err != nil {
		return fmt.Errorf("create exec for ollama pull: %w", err)
	}

	return cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{Detach: true})
}

func ContainerStatus(ctx context.Context, cli *client.Client, name string) (string, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", name)),
	})
	if err != nil {
		return "", err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if n == "/"+name {
				return c.State, nil
			}
		}
	}
	return "not_found", nil
}

func EnsureInfraContainers(ctx context.Context, cli *client.Client, hostPort string) error {
	if err := EnsureNetwork(ctx, cli); err != nil {
		return err
	}
	if err := EnsurePostgresContainer(ctx, cli, hostPort); err != nil {
		return err
	}
	if err := EnsureOllamaContainer(ctx, cli); err != nil {
		return err
	}
	return nil
}

func ContainerLogs(ctx context.Context, cli *client.Client, name string, follow bool, tail string) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
	}
	if tail != "" && tail != "all" {
		options.Tail = tail
	}
	return cli.ContainerLogs(ctx, name, options)
}

// ImageExists returns true if the image is present in the local Docker cache.
func ImageExists(ctx context.Context, cli *client.Client, ref string) bool {
	_, err := cli.ImageInspect(ctx, ref)
	return err == nil
}

// TagImage adds a new tag to an existing local image.
func TagImage(ctx context.Context, cli *client.Client, source, target string) error {
	return cli.ImageTag(ctx, source, target)
}

// IsPortFree returns true if nothing is listening on the given TCP port on localhost.
func IsPortFree(port string) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

// FindFreePort returns the preferred port if it is free, otherwise asks the OS
// for any available port and returns that.
func FindFreePort(preferred string) (string, error) {
	if IsPortFree(preferred) {
		return preferred, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("find free port: %w", err)
	}
	defer ln.Close()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "", fmt.Errorf("parse port: %w", err)
	}
	return port, nil
}

func CheckHTTP(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}


// daemonDockerfile is the combined image that runs the akama daemon (as root)
// and the agent CLIs (claude, opencode) as the non-root worker user (uid 1000).
// The daemon drops privileges to uid/gid 1000 before exec-ing agent processes.
const daemonDockerfile = `FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git docker.io nodejs npm curl bash \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 -s /bin/bash worker
USER worker
ENV NPM_CONFIG_PREFIX=/home/worker/.npm-global
ENV PATH="/home/worker/.npm-global/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
RUN npm install -g @anthropic-ai/claude-code opencode-ai
USER root
COPY akama /usr/local/bin/akama
WORKDIR /workspaces
ENTRYPOINT ["akama", "--daemon"]
`

// BuildDaemonImage cross-compiles the daemon for Linux, then packages it into
// the akama-daemon Docker image. binaryPath is used only to locate the project root.
// version is baked into the binary via ldflags; pass "" to use "dev".
func BuildDaemonImage(ctx context.Context, cli *client.Client, binaryPath, version string, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintln(out, "Cross-compiling daemon for Linux...")
	linuxBin, cleanup, err := crossCompileForLinux(binaryPath, version)
	if err != nil {
		return fmt.Errorf("cross-compile: %w", err)
	}
	defer cleanup()

	binaryData, err := os.ReadFile(linuxBin)
	if err != nil {
		return fmt.Errorf("read compiled binary: %w", err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	dockerfileContent := []byte(daemonDockerfile)
	if err := tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Size: int64(len(dockerfileContent)),
		Mode: 0644,
	}); err != nil {
		return fmt.Errorf("write dockerfile header: %w", err)
	}
	if _, err := tw.Write(dockerfileContent); err != nil {
		return fmt.Errorf("write dockerfile: %w", err)
	}

	if err := tw.WriteHeader(&tar.Header{
		Name: "akama",
		Size: int64(len(binaryData)),
		Mode: 0755,
	}); err != nil {
		return fmt.Errorf("write binary header: %w", err)
	}
	if _, err := tw.Write(binaryData); err != nil {
		return fmt.Errorf("write binary: %w", err)
	}
	tw.Close()

	resp, err := cli.ImageBuild(ctx, &buf, build.ImageBuildOptions{
		Tags:       []string{DaemonImage},
		Dockerfile: "Dockerfile",
		Remove:     true,
	})
	if err != nil {
		return fmt.Errorf("image build: %w", err)
	}
	defer resp.Body.Close()

	fd, isTerminal := termFD(out)
	return jsonmessage.DisplayJSONMessagesStream(resp.Body, out, fd, isTerminal, nil)
}

// crossCompileForLinux compiles the daemon binary for the current CPU arch on
// Linux. It reads OAuth credentials from the .env file in the project root (if
// present) so the resulting binary has the same baked-in credentials as a
// make build. Returns the path of the compiled binary and a cleanup function.
func crossCompileForLinux(binaryPath, version string) (string, func(), error) {
	projectRoot, err := findProjectRoot(binaryPath)
	if err != nil {
		return "", nil, err
	}

	tmp, err := os.CreateTemp("", "akama-linux-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()
	cleanup := func() { os.Remove(tmp.Name()) }

	ldflags := daemonLDFlags(projectRoot, version)
	args := []string{"build", "-o", tmp.Name(), "-trimpath", "-ldflags", ldflags, "."}

	cmd := exec.Command("go", args...)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(),
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
		"CGO_ENABLED=0",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("go build: %w\n%s", err, output)
	}
	return tmp.Name(), cleanup, nil
}

// findProjectRoot walks up from binaryPath and then from cwd looking for go.mod.
func findProjectRoot(binaryPath string) (string, error) {
	candidates := []string{filepath.Dir(binaryPath)}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("go.mod not found near %s or cwd — run akama from the project root", binaryPath)
}

// daemonLDFlags builds the -ldflags string for the daemon binary, reading
// OAuth credentials from .env in the project root when present.
func daemonLDFlags(projectRoot, version string) string {
	env := parseDotEnv(filepath.Join(projectRoot, ".env"))
	const pkg = "github.com/jullury/akama/internal/config"
	var flags []string
	for _, p := range [][2]string{
		{"GITHUB_CLIENT_ID", pkg + ".GitHubClientID"},
		{"GITHUB_CLIENT_SECRET", pkg + ".GitHubClientSecret"},
		{"GITLAB_CLIENT_ID", pkg + ".GitLabClientID"},
		{"GITLAB_CLIENT_SECRET", pkg + ".GitLabClientSecret"},
	} {
		if v := env[p[0]]; v != "" {
			flags = append(flags, fmt.Sprintf("-X %s=%s", p[1], v))
		}
	}
	if version != "" && version != "dev" {
		flags = append(flags, fmt.Sprintf("-X %s.Version=%s", pkg, version))
	}
	return strings.Join(flags, " ")
}

// parseDotEnv reads a .env file and returns a key→value map.
func parseDotEnv(path string) map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if ok {
			result[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return result
}

// EnsureDaemonContainer creates and starts the akama-daemon container if it is not
// already running. It always recreates stopped containers to pick up config changes.
// Workspaces are served from the named volume WorkspacesVolume mounted at /workspaces.
func EnsureDaemonContainer(ctx context.Context, cli *client.Client, configDir, logsDir string) error {
	healthy, _ := ContainerHealthy(ctx, cli, DaemonContainer)
	if healthy {
		return nil
	}

	// Force-remove any existing container (running or stopped) so we always
	// start fresh — this replaces crash-looping containers and picks up new
	// images / env vars.
	if err := RemoveContainer(ctx, cli, DaemonContainer); err != nil {
		return fmt.Errorf("remove stale daemon container: %w", err)
	}

	// Ensure bind-mount directories exist on the host.
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	homeDir, _ := os.UserHomeDir()

	claudeDir := filepath.Join(homeDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	mounts := []mount.Mount{
		{
			Type:   mount.TypeBind,
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		},
		// Named volume for workspaces.
		{
			Type:   mount.TypeVolume,
			Source: WorkspacesVolume,
			Target: "/workspaces",
		},
		// Mount the whole ~/.akama dir so the keyfile, config, and logs all persist.
		{
			Type:   mount.TypeBind,
			Source: configDir,
			Target: "/root/.akama",
		},
		// Mount the host ~/.claude so the agent CLIs find their auth tokens.
		{
			Type:   mount.TypeBind,
			Source: claudeDir,
			Target: "/home/worker/.claude",
		},
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: DaemonImage,
		Env: []string{
			"POSTGRES_URL=" + InternalPostgresURL,
			"OLLAMA_URL=" + InternalOllamaURL,
		},
		Labels: map[string]string{"app": "akama"},
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		Mounts:        mounts,
	}, nil, nil, DaemonContainer)
	if err != nil {
		return fmt.Errorf("create daemon container: %w", err)
	}

	if err := cli.NetworkConnect(ctx, NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect daemon to network: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start daemon container: %w", err)
	}

	return nil
}

