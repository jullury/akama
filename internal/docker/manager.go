package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/go-connections/nat"
)

const (
	NetworkName        = "akama-net"
	PostgresContainer  = "akama-postgres"
	OllamaContainer    = "akama-ollama"
	DaemonContainer    = "akama-daemon"
	PostgresImage      = "pgvector/pgvector:pg16"
	OllamaImage        = "ollama/ollama"
	DaemonImage        = "akama-daemon:latest"
	PostgresURL        = "postgres://akama:akama@akama-postgres:5432/akama"
	PostgresHostURL    = "postgres://akama:akama@127.0.0.1"
	PostgresHostPort   = "5432"
	OllamaURL          = "http://akama-ollama:11434"
)

func NewClient() (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
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

func PullImage(ctx context.Context, cli *client.Client, imageRef string, out io.Writer) error {
	reader, err := cli.ImagePull(ctx, imageRef, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", imageRef, err)
	}
	defer reader.Close()
	if out != nil {
		return jsonmessage.DisplayJSONMessagesStream(reader, out, 0, false, nil)
	}
	io.Copy(io.Discard, reader)
	return nil
}

func BuildImage(ctx context.Context, cli *client.Client, dockerfilePath, tag string, buildArgs map[string]*string, out io.Writer) error {
	tarBuf, err := tarBuildContext(dockerfilePath)
	if err != nil {
		return fmt.Errorf("create build context: %w", err)
	}

	opts := types.ImageBuildOptions{
		Tags:        []string{tag},
		Dockerfile:  filepath.Base(dockerfilePath),
		BuildArgs:   buildArgs,
		Remove:      true,
		ForceRemove: true,
	}
	reader, err := cli.ImageBuild(ctx, tarBuf, opts)
	if err != nil {
		return fmt.Errorf("build image: %w", err)
	}
	defer reader.Body.Close()
	if out != nil {
		return jsonmessage.DisplayJSONMessagesStream(reader.Body, out, 0, false, nil)
	}
	io.Copy(io.Discard, reader.Body)
	return nil
}

func tarBuildContext(dockerfilePath string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	df, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer df.Close()

	dfInfo, err := df.Stat()
	if err != nil {
		return nil, err
	}

	header := &tar.Header{
		Name:     filepath.Base(dockerfilePath),
		Size:     dfInfo.Size(),
		Mode:     0644,
		ModTime:  dfInfo.ModTime(),
		AccessTime: dfInfo.ModTime(),
		ChangeTime: dfInfo.ModTime(),
	}
	if err := tw.WriteHeader(header); err != nil {
		return nil, err
	}
	if _, err := io.Copy(tw, df); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

func EnsurePostgresContainer(ctx context.Context, cli *client.Client, hostPort string) error {
	exists, err := ContainerExists(ctx, cli, PostgresContainer)
	if err != nil {
		return err
	}
	if exists {
		return nil
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
	exists, err := ContainerExists(ctx, cli, OllamaContainer)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:  OllamaImage,
		Labels: map[string]string{"app": "akama"},
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
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

func EnsureDaemonContainer(ctx context.Context, cli *client.Client, hostWorkspaceDir, configPath, logDir string) error {
	exists, err := ContainerExists(ctx, cli, DaemonContainer)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	homeDir, _ := os.UserHomeDir()
	absWorkspace := expandPath(hostWorkspaceDir, homeDir)
	absConfig := expandPath(configPath, homeDir)
	absLog := expandPath(logDir, homeDir)

	binds := []string{
		absWorkspace + ":/workspaces",
		absConfig + ":/root/.akama/config.yaml",
		absLog + ":/root/.akama/logs",
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: DaemonImage,
		Env: []string{
			"POSTGRES_URL=" + PostgresURL,
			"OLLAMA_URL=" + OllamaURL,
		},
		Labels: map[string]string{"app": "akama"},
	}, &container.HostConfig{
		Binds:         binds,
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
	}, nil, nil, DaemonContainer)
	if err != nil {
		return fmt.Errorf("create daemon container: %w", err)
	}

	if err := cli.NetworkConnect(ctx, NetworkName, resp.ID, nil); err != nil {
		return fmt.Errorf("connect daemon to network: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start daemon: %w", err)
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

func PullAndEnsureModel(ctx context.Context, cli *client.Client, model string) error {
	execResp, err := cli.ContainerExecCreate(ctx, OllamaContainer, container.ExecOptions{
		Cmd:          []string{"ollama", "pull", model},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("create exec for ollama pull: %w", err)
	}

	resp, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("attach to ollama pull: %w", err)
	}
	defer resp.Close()

	io.Copy(io.Discard, resp.Reader)
	return nil
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

func EnsureContainers(ctx context.Context, cli *client.Client, hostWorkspaceDir, configPath, logDir, hostPort string) error {
	if err := EnsureNetwork(ctx, cli); err != nil {
		return err
	}
	if err := EnsurePostgresContainer(ctx, cli, hostPort); err != nil {
		return err
	}
	if err := EnsureOllamaContainer(ctx, cli); err != nil {
		return err
	}
	if err := EnsureDaemonContainer(ctx, cli, hostWorkspaceDir, configPath, logDir); err != nil {
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

func expandPath(path, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}
