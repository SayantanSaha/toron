package version

import (
	"strings"
	"testing"
)

func TestVersionFunctions(t *testing.T) {
	v := Get()
	if v == "" {
		t.Error("expected non-empty version string")
	}

	short := ShortString()
	if !strings.HasPrefix(short, "v") {
		t.Errorf("expected ShortString to start with 'v', got %s", short)
	}

	full := Full()
	if !strings.Contains(full, "Toron") {
		t.Errorf("expected Full() to contain 'Toron', got %s", full)
	}

	info := GetInfo()
	if info.Version != v {
		t.Errorf("expected info.Version == %s, got %s", v, info.Version)
	}
	if info.Platform == "" {
		t.Error("expected non-empty platform")
	}
}
