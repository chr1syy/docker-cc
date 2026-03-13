package docker

import (
    "testing"
    "time"
)

func TestParseLogLine_Empty(t *testing.T) {
    ts, stream, msg := ParseLogLine([]byte{})
    if ts != "" || stream != "stdout" || msg != "" {
        t.Errorf("expected empty result, got ts=%q stream=%q msg=%q", ts, stream, msg)
    }
}

func TestParseLogLine_MultiplexedStdout(t *testing.T) {
    // Build a multiplexed frame: stream=1(stdout), 3 zero bytes, 4-byte length, then payload
    payload := []byte("2024-01-15T10:30:00.123456789Z hello world")
    frame := make([]byte, 8+len(payload))
    frame[0] = 1 // stdout
    frame[4] = byte(len(payload) >> 24)
    frame[5] = byte(len(payload) >> 16)
    frame[6] = byte(len(payload) >> 8)
    frame[7] = byte(len(payload))
    copy(frame[8:], payload)

    ts, stream, msg := ParseLogLine(frame)
    if stream != "stdout" {
        t.Errorf("expected stdout, got %q", stream)
    }
    if ts != "2024-01-15T10:30:00.123456789Z" {
        t.Errorf("expected timestamp, got %q", ts)
    }
    if msg != "hello world" {
        t.Errorf("expected 'hello world', got %q", msg)
    }
}

func TestParseLogLine_MultiplexedStderr(t *testing.T) {
    payload := []byte("2024-01-15T10:30:00Z error occurred")
    frame := make([]byte, 8+len(payload))
    frame[0] = 2 // stderr
    frame[7] = byte(len(payload))
    copy(frame[8:], payload)

    ts, stream, msg := ParseLogLine(frame)
    if stream != "stderr" {
        t.Errorf("expected stderr, got %q", stream)
    }
    if ts != "2024-01-15T10:30:00Z" {
        t.Errorf("expected timestamp, got %q", ts)
    }
    if msg != "error occurred" {
        t.Errorf("expected 'error occurred', got %q", msg)
    }
}

func TestParseLogLine_MultiplexedNoTimestamp(t *testing.T) {
    payload := []byte("just a message")
    frame := make([]byte, 8+len(payload))
    frame[0] = 1
    frame[7] = byte(len(payload))
    copy(frame[8:], payload)

    ts, stream, msg := ParseLogLine(frame)
    if ts != "" {
        t.Errorf("expected empty timestamp, got %q", ts)
    }
    if stream != "stdout" {
        t.Errorf("expected stdout, got %q", stream)
    }
    if msg != "just a message" {
        t.Errorf("expected 'just a message', got %q", msg)
    }
}

func TestParseLogLine_RawTTYWithTimestamp(t *testing.T) {
    line := []byte("2024-01-15T10:30:00.123456789Z raw tty output")
    ts, stream, msg := ParseLogLine(line)
    if stream != "stdout" {
        t.Errorf("expected stdout, got %q", stream)
    }
    if ts != "2024-01-15T10:30:00.123456789Z" {
        t.Errorf("expected timestamp, got %q", ts)
    }
    if msg != "raw tty output" {
        t.Errorf("expected 'raw tty output', got %q", msg)
    }
}

func TestParseLogLine_RawTTYNoTimestamp(t *testing.T) {
    line := []byte("plain log line without timestamp")
    ts, stream, msg := ParseLogLine(line)
    if ts != "" {
        t.Errorf("expected empty timestamp, got %q", ts)
    }
    if stream != "stdout" {
        t.Errorf("expected stdout, got %q", stream)
    }
    if msg != "plain log line without timestamp" {
        t.Errorf("expected full line as message, got %q", msg)
    }
}

func TestParseLogLine_RFC3339Timestamp(t *testing.T) {
    payload := []byte("2024-01-15T10:30:00Z message")
    frame := make([]byte, 8+len(payload))
    frame[0] = 1
    frame[7] = byte(len(payload))
    copy(frame[8:], payload)

    ts, _, _ := ParseLogLine(frame)
    // Verify it parses as valid time
    _, err := time.Parse(time.RFC3339, ts)
    if err != nil {
        t.Errorf("timestamp %q is not valid RFC3339: %v", ts, err)
    }
}
