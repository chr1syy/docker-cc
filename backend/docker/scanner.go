package docker

import (
    "bufio"
    "context"
    "io"
    "regexp"
    "time"
)

// LogLevel categorizes a digest line.
type LogLevel string

const (
    LevelError LogLevel = "error"
    LevelWarn  LogLevel = "warn"
)

// DigestLine is a single sampled log entry retained by ScanLogs.
type DigestLine struct {
    Timestamp string   `json:"ts"`
    Stream    string   `json:"stream"` // "stdout" | "stderr"
    Level     LogLevel `json:"level"`
    Text      string   `json:"text"`
}

// DigestResult is the aggregated outcome of a single ScanLogs invocation.
type DigestResult struct {
    ErrorCount   int          `json:"error_count"`
    WarnCount    int          `json:"warn_count"`
    FirstErrorAt string       `json:"first_error_at,omitempty"`
    LastErrorAt  string       `json:"last_error_at,omitempty"`
    Samples      []DigestLine `json:"samples"`
    Truncated    bool         `json:"truncated"`
    Scanned      int          `json:"lines_scanned"`
}

// DigestOptions tunes ScanLogs behavior. Zero values use defaults applied
// inside ScanLogs (SampleLimit=50, MaxScan=50000). ErrorRegex is required;
// callers should pass DefaultErrorRegex when unsure. WarnRegex may be nil
// to disable warn counting.
type DigestOptions struct {
    Since       time.Duration
    ErrorRegex  *regexp.Regexp
    WarnRegex   *regexp.Regexp
    SampleLimit int
    MaxScan     int
}

// Default regexes used when callers don't supply their own.
var (
    DefaultErrorRegex = regexp.MustCompile(`(?i)\b(error|fatal|panic|exception|traceback)\b`)
    DefaultWarnRegex  = regexp.MustCompile(`(?i)\bwarn(ing)?\b`)
)

// Per-sample text length cap. Samples beyond this length are truncated and
// have a single ellipsis rune appended. Test scanner_test.go pins the rule:
// truncation is rune-based — the resulting Text contains exactly maxSampleLen
// runes plus one "…" rune (utf8.RuneCountInString == maxSampleLen+1).
const maxSampleLen = 2000

// LogsSince fetches container logs covering the trailing `since` window.
// Returns a ReadCloser the caller must close. Internally calls
// GetContainerLogs with Tail: 0 (unbounded — Since is the bound) and
// timestamps enabled so ParseLogLine can extract per-line timestamps.
func (d *Client) LogsSince(ctx context.Context, containerID string, since time.Duration) (io.ReadCloser, error) {
    tsPtr := new(bool)
    *tsPtr = true
    opts := LogOptions{
        Since:      time.Now().Add(-since).Format(time.RFC3339),
        Tail:       0,
        Follow:     false,
        Timestamps: tsPtr,
    }
    return d.GetContainerLogs(ctx, containerID, opts)
}

// ScanLogs reads a Docker log stream line-by-line through opts.ErrorRegex
// (and optionally opts.WarnRegex), returning counts plus bounded samples.
// It never returns an error — best-effort, silent on read failures.
//
// Sampling: error samples are preferred; if fewer than SampleLimit error
// lines were seen, the remainder is filled with warn lines. Total samples
// are capped at SampleLimit. Errors take precedence over warns when a line
// matches both regexes — that line is counted only as an error.
//
// Safety: a hard cap of MaxScan lines is enforced; when reached, Truncated
// is set and scanning stops.
func ScanLogs(rc io.Reader, opts DigestOptions) DigestResult {
    if opts.SampleLimit <= 0 {
        opts.SampleLimit = 50
    }
    if opts.MaxScan <= 0 {
        opts.MaxScan = 50000
    }

    res := DigestResult{Samples: []DigestLine{}}
    if rc == nil || opts.ErrorRegex == nil {
        return res
    }

    scanner := bufio.NewScanner(rc)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

    errorSamples := make([]DigestLine, 0, opts.SampleLimit)
    warnSamples := make([]DigestLine, 0, opts.SampleLimit)

    for scanner.Scan() {
        res.Scanned++

        line := scanner.Bytes()
        ts, stream, msg := ParseLogLine(line)

        isError := opts.ErrorRegex.MatchString(msg)
        isWarn := !isError && opts.WarnRegex != nil && opts.WarnRegex.MatchString(msg)

        switch {
        case isError:
            res.ErrorCount++
            if res.FirstErrorAt == "" {
                res.FirstErrorAt = ts
            }
            res.LastErrorAt = ts
            if len(errorSamples) < opts.SampleLimit {
                errorSamples = append(errorSamples, DigestLine{
                    Timestamp: ts,
                    Stream:    stream,
                    Level:     LevelError,
                    Text:      clampSampleText(msg),
                })
            }
        case isWarn:
            res.WarnCount++
            if len(warnSamples) < opts.SampleLimit {
                warnSamples = append(warnSamples, DigestLine{
                    Timestamp: ts,
                    Stream:    stream,
                    Level:     LevelWarn,
                    Text:      clampSampleText(msg),
                })
            }
        }

        if res.Scanned >= opts.MaxScan {
            res.Truncated = true
            break
        }
    }

    samples := errorSamples
    if len(samples) < opts.SampleLimit {
        remaining := opts.SampleLimit - len(samples)
        if remaining > len(warnSamples) {
            remaining = len(warnSamples)
        }
        samples = append(samples, warnSamples[:remaining]...)
    }
    res.Samples = samples

    return res
}

// clampSampleText truncates s to at most maxSampleLen runes, appending a
// single "…" rune when truncated. Rune-based so multi-byte UTF-8 sequences
// are never split mid-rune.
func clampSampleText(s string) string {
    runes := []rune(s)
    if len(runes) <= maxSampleLen {
        return s
    }
    return string(runes[:maxSampleLen]) + "…"
}
