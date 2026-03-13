package handlers

import (
    "testing"

    "github.com/docker/go-connections/nat"
)

func TestFormatPorts_WithBindings(t *testing.T) {
    pm := nat.PortMap{
        "80/tcp": []nat.PortBinding{
            {HostIP: "0.0.0.0", HostPort: "8080"},
        },
    }
    ports := formatPorts(pm)
    if len(ports) != 1 {
        t.Fatalf("expected 1 port, got %d", len(ports))
    }
    expected := "0.0.0.0:8080->80/tcp"
    if ports[0] != expected {
        t.Errorf("expected %q, got %q", expected, ports[0])
    }
}

func TestFormatPorts_NoBindings(t *testing.T) {
    pm := nat.PortMap{
        "3306/tcp": nil,
    }
    ports := formatPorts(pm)
    if len(ports) != 1 {
        t.Fatalf("expected 1 port, got %d", len(ports))
    }
    expected := "3306/tcp"
    if ports[0] != expected {
        t.Errorf("expected %q, got %q", expected, ports[0])
    }
}

func TestFormatPorts_EmptyBindings(t *testing.T) {
    pm := nat.PortMap{
        "443/tcp": []nat.PortBinding{},
    }
    ports := formatPorts(pm)
    if len(ports) != 1 {
        t.Fatalf("expected 1 port, got %d", len(ports))
    }
    if ports[0] != "443/tcp" {
        t.Errorf("expected '443/tcp', got %q", ports[0])
    }
}

func TestFormatPorts_MultipleBindings(t *testing.T) {
    pm := nat.PortMap{
        "80/tcp": []nat.PortBinding{
            {HostIP: "0.0.0.0", HostPort: "8080"},
            {HostIP: "::", HostPort: "8080"},
        },
    }
    ports := formatPorts(pm)
    if len(ports) != 2 {
        t.Fatalf("expected 2 ports, got %d", len(ports))
    }
}

func TestFormatPorts_EmptyMap(t *testing.T) {
    ports := formatPorts(nat.PortMap{})
    if len(ports) != 0 {
        t.Errorf("expected empty ports, got %d", len(ports))
    }
}

func TestFormatPorts_BindingNoHostPort(t *testing.T) {
    pm := nat.PortMap{
        "9090/tcp": []nat.PortBinding{
            {HostIP: "", HostPort: ""},
        },
    }
    ports := formatPorts(pm)
    if len(ports) != 1 {
        t.Fatalf("expected 1 port, got %d", len(ports))
    }
    if ports[0] != "9090/tcp" {
        t.Errorf("expected '9090/tcp', got %q", ports[0])
    }
}

func TestRedactEnv(t *testing.T) {
    env := []string{
        "PATH=/usr/bin",
        "SECRET=mysecretvalue",
        "EMPTY=",
        "NOEQUALS",
    }
    redacted := redactEnv(env)
    if len(redacted) != 4 {
        t.Fatalf("expected 4 entries, got %d", len(redacted))
    }
    if redacted[0] != "PATH=REDACTED" {
        t.Errorf("expected PATH=REDACTED, got %q", redacted[0])
    }
    if redacted[1] != "SECRET=REDACTED" {
        t.Errorf("expected SECRET=REDACTED, got %q", redacted[1])
    }
    // "EMPTY=" has = at index 5, which is > 0, so it gets redacted
    if redacted[2] != "EMPTY=REDACTED" {
        t.Errorf("expected EMPTY=REDACTED, got %q", redacted[2])
    }
    if redacted[3] != "NOEQUALS" {
        t.Errorf("expected NOEQUALS unchanged, got %q", redacted[3])
    }
}

func TestRedactEnv_Empty(t *testing.T) {
    redacted := redactEnv([]string{})
    if len(redacted) != 0 {
        t.Errorf("expected empty slice, got %d entries", len(redacted))
    }
}
