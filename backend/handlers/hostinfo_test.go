package handlers

import (
	"math"
	"testing"
)

func TestHostInfo_ParseLoadAvg(t *testing.T) {
	load1, load5, load15 := parseLoadAvg("0.11 0.09 0.05 1/523 12345\n")
	if load1 != 0.11 {
		t.Errorf("load1: expected 0.11, got %v", load1)
	}
	if load5 != 0.09 {
		t.Errorf("load5: expected 0.09, got %v", load5)
	}
	if load15 != 0.05 {
		t.Errorf("load15: expected 0.05, got %v", load15)
	}
}

func TestHostInfo_ParseLoadAvg_Malformed(t *testing.T) {
	// Fewer than three fields must not panic; missing fields stay zero.
	load1, load5, load15 := parseLoadAvg("0.42")
	if load1 != 0.42 {
		t.Errorf("load1: expected 0.42, got %v", load1)
	}
	if load5 != 0 || load15 != 0 {
		t.Errorf("expected load5/load15 to be 0, got %v/%v", load5, load15)
	}

	l1, l5, l15 := parseLoadAvg("")
	if l1 != 0 || l5 != 0 || l15 != 0 {
		t.Errorf("empty input should yield zeros, got %v/%v/%v", l1, l5, l15)
	}
}

func TestHostInfo_ParseMemInfo(t *testing.T) {
	raw := `MemTotal:        8060092 kB
MemFree:          123456 kB
MemAvailable:    6291456 kB
Buffers:           98765 kB
Cached:          1234567 kB
`
	total, avail, usedPct := parseMemInfo(raw)

	wantTotal := uint64(8060092) * 1024
	if total != wantTotal {
		t.Errorf("total: expected %d, got %d", wantTotal, total)
	}
	wantAvail := uint64(6291456) * 1024
	if avail != wantAvail {
		t.Errorf("available: expected %d, got %d", wantAvail, avail)
	}
	// used = (1 - 6291456/8060092) * 100 ≈ 21.94
	wantUsed := (1 - float64(6291456)/float64(8060092)) * 100
	if math.Abs(usedPct-wantUsed) > 1e-9 {
		t.Errorf("used percent: expected %v, got %v", wantUsed, usedPct)
	}
}

func TestHostInfo_ParseMemInfo_MissingTotalNoDivideByZero(t *testing.T) {
	// Without MemTotal the used percentage must be 0, not NaN/Inf.
	total, avail, usedPct := parseMemInfo("MemAvailable:  1000 kB\n")
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if avail != uint64(1000)*1024 {
		t.Errorf("expected available %d, got %d", uint64(1000)*1024, avail)
	}
	if usedPct != 0 {
		t.Errorf("expected used percent 0 when total unknown, got %v", usedPct)
	}
}
