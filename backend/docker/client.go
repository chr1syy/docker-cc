package docker

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "time"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/client"
)

type Client struct {
    c *client.Client
}

func New() (*Client, error) {
    c, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return nil, err
    }
    return &Client{c: c}, nil
}

func (d *Client) Close() error {
    if d.c == nil {
        return nil
    }
    return d.c.Close()
}

// Ping checks connectivity with the Docker daemon. It returns nil when
// the daemon responds successfully.
func (d *Client) Ping(ctx context.Context) error {
    if d == nil || d.c == nil {
        return fmt.Errorf("docker client unavailable")
    }
    _, err := d.c.Ping(ctx)
    return err
}

func (d *Client) ListContainers(ctx context.Context) ([]types.Container, error) {
    return d.c.ContainerList(ctx, container.ListOptions{All: true})
}

func (d *Client) InspectContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
    return d.c.ContainerInspect(ctx, id)
}

func (d *Client) ContainerStats(ctx context.Context, id string) (types.ContainerStats, error) {
    // Caller must close the returned Body if used; here we return the SDK type
    return d.c.ContainerStats(ctx, id, false)
}

// StartContainer starts the given container.
func (d *Client) StartContainer(ctx context.Context, id string) error {
    if d == nil || d.c == nil {
        return fmt.Errorf("docker client unavailable")
    }
    return d.c.ContainerStart(ctx, id, container.StartOptions{})
}

// StopContainer stops the given container with a graceful timeout.
// timeout is the duration to wait before killing the container.
func (d *Client) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
    if d == nil || d.c == nil {
        return fmt.Errorf("docker client unavailable")
    }
    timeoutSecs := int(timeout.Seconds())
    return d.c.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeoutSecs})
}

// RestartContainer restarts the given container with the provided timeout.
func (d *Client) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
    if d == nil || d.c == nil {
        return fmt.Errorf("docker client unavailable")
    }
    timeoutSecs := int(timeout.Seconds())
    return d.c.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeoutSecs})
}

// RemoveContainer removes a stopped container.
func (d *Client) RemoveContainer(ctx context.Context, id string) error {
    if d == nil || d.c == nil {
        return fmt.Errorf("docker client unavailable")
    }
    return d.c.ContainerRemove(ctx, id, container.RemoveOptions{})
}

// LogOptions controls log retrieval behavior.
type LogOptions struct {
    Since      string  // RFC3339 timestamp
    Until      string  // RFC3339 timestamp
    Tail       int     // number of lines (default 500 when zero)
    Follow     bool    // stream
    Timestamps *bool   // include timestamps (default true when nil)
}

// GetContainerLogs returns a ReadCloser from the Docker daemon for the
// requested container logs. Caller must close the returned reader.
func (d *Client) GetContainerLogs(ctx context.Context, containerID string, opts LogOptions) (io.ReadCloser, error) {
    if d == nil || d.c == nil {
        return nil, fmt.Errorf("docker client unavailable")
    }

    tail := "500"
    if opts.Tail != 0 {
        tail = fmt.Sprintf("%d", opts.Tail)
    }

    // default timestamps to true unless caller explicitly sets false
    timestamps := true
    if opts.Timestamps != nil {
        timestamps = *opts.Timestamps
    }

    lopts := container.LogsOptions{
        ShowStdout: true,
        ShowStderr: true,
        Since:      opts.Since,
        Until:      opts.Until,
        Timestamps: timestamps,
        Tail:       tail,
        Follow:     opts.Follow,
        Details:    false,
    }

    return d.c.ContainerLogs(ctx, containerID, lopts)
}

// ParseLogLine parses a single frame from the Docker logs stream and
// returns (timestamp, stream, message). It handles both the 8-byte
// multiplexed header (non-tty) and raw tty output. timestamp will be
// empty when no parsable timestamp is present.
func ParseLogLine(line []byte) (string, string, string) {
    if len(line) == 0 {
        return "", "stdout", ""
    }

    // Check for the Docker multiplexed header: 1 byte stream, 3 bytes
    // unused, 4 bytes big-endian payload length
    if len(line) >= 8 {
        st := line[0]
        if st == 1 || st == 2 {
            // payload length is present but we don't strictly require it
            // to match since reads may coalesce frames. Extract payload.
            payload := line[8:]
            stream := "stdout"
            if st == 2 {
                stream = "stderr"
            }
            // try to split timestamp (RFC3339/RFC3339Nano) from message
            if i := bytes.IndexByte(payload, ' '); i > 0 {
                tsCand := string(payload[:i])
                if _, err := time.Parse(time.RFC3339Nano, tsCand); err == nil {
                    return tsCand, stream, string(payload[i+1:])
                }
                if _, err := time.Parse(time.RFC3339, tsCand); err == nil {
                    return tsCand, stream, string(payload[i+1:])
                }
            }
            return "", stream, string(payload)
        }
    }

    // Fallback: raw tty output (no header). Treat as stdout and try to
    // parse a leading timestamp if present.
    if i := bytes.IndexByte(line, ' '); i > 0 {
        tsCand := string(line[:i])
        if _, err := time.Parse(time.RFC3339Nano, tsCand); err == nil {
            return tsCand, "stdout", string(line[i+1:])
        }
        if _, err := time.Parse(time.RFC3339, tsCand); err == nil {
            return tsCand, "stdout", string(line[i+1:])
        }
    }

    return "", "stdout", string(line)
}
