package docker

import (
    "encoding/binary"
    "strings"
    "testing"
    "unicode/utf8"
)

// frame builds a single Docker multiplexed-log frame: 1 byte stream marker,
// 3 zero bytes, 4-byte big-endian payload length, then the payload bytes.
// Callers should include a trailing "\n" inside payload — bufio.Scanner
// splits frames on newlines.
func frame(stream byte, payload string) string {
    b := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
    binary.BigEndian.PutUint32(b[4:8], uint32(len(payload)))
    return string(b) + payload
}

// stdoutFrame is shorthand for frame(0x01, ...) with a trailing newline.
func stdoutFrame(payload string) string {
    return frame(1, payload+"\n")
}

// stderrFrame is shorthand for frame(0x02, ...) with a trailing newline.
func stderrFrame(payload string) string {
    return frame(2, payload+"\n")
}

func TestScanLogs(t *testing.T) {
    t.Run("error_lines_counted", func(t *testing.T) {
        input := stdoutFrame("2026-05-17T10:00:00Z ERROR boom one") +
            stdoutFrame("2026-05-17T10:00:01Z plain noise") +
            stdoutFrame("2026-05-17T10:00:02Z ERROR boom two") +
            stdoutFrame("2026-05-17T10:00:03Z another plain line") +
            stdoutFrame("2026-05-17T10:00:04Z panic: bad thing happened")

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
        })

        if res.ErrorCount != 3 {
            t.Errorf("expected ErrorCount=3, got %d", res.ErrorCount)
        }
        if res.Scanned != 5 {
            t.Errorf("expected Scanned=5, got %d", res.Scanned)
        }
        if res.Truncated {
            t.Errorf("expected Truncated=false, got true")
        }
    })

    t.Run("warn_and_error_distinct", func(t *testing.T) {
        input := stdoutFrame("2026-05-17T10:00:00Z ERROR something failed") +
            stdoutFrame("2026-05-17T10:00:01Z WARN something slow") +
            stdoutFrame("2026-05-17T10:00:02Z plain informational line")

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
            WarnRegex:  DefaultWarnRegex,
        })

        if res.ErrorCount != 1 {
            t.Errorf("expected ErrorCount=1, got %d", res.ErrorCount)
        }
        if res.WarnCount != 1 {
            t.Errorf("expected WarnCount=1, got %d", res.WarnCount)
        }
    })

    t.Run("error_takes_precedence_over_warn", func(t *testing.T) {
        // Line that matches both regexes — must count as error only.
        input := stdoutFrame("2026-05-17T10:00:00Z ERROR with a warning word inside")

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
            WarnRegex:  DefaultWarnRegex,
        })

        if res.ErrorCount != 1 {
            t.Errorf("expected ErrorCount=1, got %d", res.ErrorCount)
        }
        if res.WarnCount != 0 {
            t.Errorf("expected WarnCount=0 (errors take precedence), got %d", res.WarnCount)
        }
    })

    t.Run("sample_truncation", func(t *testing.T) {
        var b strings.Builder
        for i := 0; i < 100; i++ {
            b.WriteString(stdoutFrame("2026-05-17T10:00:00Z ERROR repeated failure"))
        }

        res := ScanLogs(strings.NewReader(b.String()), DigestOptions{
            ErrorRegex:  DefaultErrorRegex,
            SampleLimit: 5,
        })

        if len(res.Samples) != 5 {
            t.Errorf("expected 5 samples, got %d", len(res.Samples))
        }
        if res.ErrorCount != 100 {
            t.Errorf("expected ErrorCount=100, got %d", res.ErrorCount)
        }
    })

    t.Run("first_last_timestamp", func(t *testing.T) {
        input := stdoutFrame("2026-05-17T10:00:00Z ERROR first one") +
            stdoutFrame("2026-05-17T10:30:00Z plain line in between") +
            stdoutFrame("2026-05-17T11:00:00Z ERROR last one")

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
        })

        if res.FirstErrorAt != "2026-05-17T10:00:00Z" {
            t.Errorf("expected FirstErrorAt=2026-05-17T10:00:00Z, got %q", res.FirstErrorAt)
        }
        if res.LastErrorAt != "2026-05-17T11:00:00Z" {
            t.Errorf("expected LastErrorAt=2026-05-17T11:00:00Z, got %q", res.LastErrorAt)
        }
    })

    t.Run("max_scan_truncates", func(t *testing.T) {
        var b strings.Builder
        for i := 0; i < 100; i++ {
            b.WriteString(stdoutFrame("2026-05-17T10:00:00Z plain line content"))
        }

        res := ScanLogs(strings.NewReader(b.String()), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
            MaxScan:    10,
        })

        if !res.Truncated {
            t.Errorf("expected Truncated=true")
        }
        if res.Scanned != 10 {
            t.Errorf("expected Scanned=10, got %d", res.Scanned)
        }
    })

    t.Run("long_line_clamped", func(t *testing.T) {
        // Payload structure: "<timestamp> ERROR <5000 chars of 'a'>"
        // The message portion (after the timestamp) is what gets clamped.
        // Build a message of exactly 5000 runes so we can assert clamping.
        msg := "ERROR " + strings.Repeat("a", 5000-len("ERROR "))
        payload := "2026-05-17T10:00:00Z " + msg
        input := stdoutFrame(payload)

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
        })

        if len(res.Samples) != 1 {
            t.Fatalf("expected 1 sample, got %d", len(res.Samples))
        }
        // Rule pinned in scanner.go: clamped Text contains exactly
        // maxSampleLen (2000) runes plus a single "…" rune.
        got := utf8.RuneCountInString(res.Samples[0].Text)
        if got != 2001 {
            t.Errorf("expected sample Text to contain 2001 runes (2000 + '…'), got %d", got)
        }
        if !strings.HasSuffix(res.Samples[0].Text, "…") {
            t.Errorf("expected sample Text to end with '…', got %q", res.Samples[0].Text[len(res.Samples[0].Text)-4:])
        }
    })

    t.Run("warn_samples_fill_remainder", func(t *testing.T) {
        // 2 error lines + 5 warn lines with SampleLimit=4 should yield
        // 2 error samples followed by 2 warn samples (4 total).
        input := stdoutFrame("2026-05-17T10:00:00Z ERROR first") +
            stdoutFrame("2026-05-17T10:00:01Z ERROR second") +
            stdoutFrame("2026-05-17T10:00:02Z WARN one") +
            stdoutFrame("2026-05-17T10:00:03Z WARN two") +
            stdoutFrame("2026-05-17T10:00:04Z WARN three") +
            stdoutFrame("2026-05-17T10:00:05Z WARN four") +
            stdoutFrame("2026-05-17T10:00:06Z WARN five")

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex:  DefaultErrorRegex,
            WarnRegex:   DefaultWarnRegex,
            SampleLimit: 4,
        })

        if len(res.Samples) != 4 {
            t.Fatalf("expected 4 samples, got %d", len(res.Samples))
        }
        // First two samples are errors; remaining are warns.
        if res.Samples[0].Level != LevelError || res.Samples[1].Level != LevelError {
            t.Errorf("expected first two samples to be errors, got %q, %q",
                res.Samples[0].Level, res.Samples[1].Level)
        }
        if res.Samples[2].Level != LevelWarn || res.Samples[3].Level != LevelWarn {
            t.Errorf("expected last two samples to be warns, got %q, %q",
                res.Samples[2].Level, res.Samples[3].Level)
        }
    })

    t.Run("stream_stderr_recognized", func(t *testing.T) {
        // Verify stderr frames flow through ParseLogLine and surface in samples.
        input := stderrFrame("2026-05-17T10:00:00Z ERROR from stderr")

        res := ScanLogs(strings.NewReader(input), DigestOptions{
            ErrorRegex: DefaultErrorRegex,
        })

        if len(res.Samples) != 1 {
            t.Fatalf("expected 1 sample, got %d", len(res.Samples))
        }
        if res.Samples[0].Stream != "stderr" {
            t.Errorf("expected stream=stderr, got %q", res.Samples[0].Stream)
        }
    })

    t.Run("nil_reader_returns_empty", func(t *testing.T) {
        res := ScanLogs(nil, DigestOptions{ErrorRegex: DefaultErrorRegex})
        if res.ErrorCount != 0 || res.WarnCount != 0 || res.Scanned != 0 {
            t.Errorf("expected zero result for nil reader, got %+v", res)
        }
    })
}
