package env

import (
	"sync/atomic"
	"testing"
)

func TestGetAnnasBaseURLDefaultsWithoutResolver(t *testing.T) {
	resetResolvedEnvForTests()

	resolverCalls := int32(0)
	resolveAnnasBaseURL = func() (string, error) {
		atomic.AddInt32(&resolverCalls, 1)
		return "dynamic.example", nil
	}
	t.Cleanup(func() {
		resolveAnnasBaseURL = defaultResolveAnnasBaseURL
		resetResolvedEnvForTests()
	})

	baseURL := GetAnnasBaseURL()
	if baseURL != DefaultAnnasBaseURL {
		t.Fatalf("expected %q, got %q", DefaultAnnasBaseURL, baseURL)
	}

	if atomic.LoadInt32(&resolverCalls) != 0 {
		t.Fatalf("expected resolver not to be called by default, got %d calls", resolverCalls)
	}
}

func TestGetAnnasBaseURLUsesConfiguredBaseURLWithoutResolver(t *testing.T) {
	t.Setenv("ANNAS_BASE_URL", "fallback.example")
	resetResolvedEnvForTests()

	resolverCalls := int32(0)
	resolveAnnasBaseURL = func() (string, error) {
		atomic.AddInt32(&resolverCalls, 1)
		return "dynamic.example", nil
	}
	t.Cleanup(func() {
		resolveAnnasBaseURL = defaultResolveAnnasBaseURL
		resetResolvedEnvForTests()
	})

	baseURL := GetAnnasBaseURL()
	if baseURL != "fallback.example" {
		t.Fatalf("expected fallback.example, got %q", baseURL)
	}

	if atomic.LoadInt32(&resolverCalls) != 0 {
		t.Fatalf("expected resolver not to be called unless auto discovery is enabled, got %d calls", resolverCalls)
	}
}

func TestGetAnnasBaseURLCachesResolvedBaseURLWhenAutoEnabled(t *testing.T) {
	t.Setenv("ANNAS_AUTO_BASE_URL", "true")
	t.Setenv("ANNAS_BASE_URL", "fallback.example")

	resetResolvedEnvForTests()

	resolverCalls := int32(0)
	resolveAnnasBaseURL = func() (string, error) {
		atomic.AddInt32(&resolverCalls, 1)
		return "dynamic.example", nil
	}
	t.Cleanup(func() {
		resolveAnnasBaseURL = defaultResolveAnnasBaseURL
		resetResolvedEnvForTests()
	})

	first := GetAnnasBaseURL()
	second := GetAnnasBaseURL()

	if first != "dynamic.example" || second != "dynamic.example" {
		t.Fatalf("expected cached dynamic.example, got %q and %q", first, second)
	}

	if atomic.LoadInt32(&resolverCalls) != 1 {
		t.Fatalf("expected resolver to be called once, got %d calls", resolverCalls)
	}
}

func TestGetAnnasBaseURLFallsBackWhenAutoResolverFails(t *testing.T) {
	t.Setenv("ANNAS_AUTO_BASE_URL", "true")
	t.Setenv("ANNAS_BASE_URL", "fallback.example")

	resetResolvedEnvForTests()

	resolveAnnasBaseURL = func() (string, error) {
		return "", assertiveError("resolver failed")
	}
	t.Cleanup(func() {
		resolveAnnasBaseURL = defaultResolveAnnasBaseURL
		resetResolvedEnvForTests()
	})

	baseURL := GetAnnasBaseURL()
	if baseURL != "fallback.example" {
		t.Fatalf("expected fallback.example, got %q", baseURL)
	}
}

func TestGetEnvUsesSelectedBaseURL(t *testing.T) {
	t.Setenv("ANNAS_SECRET_KEY", "secret")
	t.Setenv("ANNAS_DOWNLOAD_PATH", t.TempDir())
	t.Setenv("ANNAS_BASE_URL", "configured.example")

	resetResolvedEnvForTests()
	t.Cleanup(resetResolvedEnvForTests)

	cfg, err := GetEnv()
	if err != nil {
		t.Fatalf("GetEnv returned error: %v", err)
	}

	if cfg.AnnasBaseURL != "configured.example" {
		t.Fatalf("expected configured.example, got %q", cfg.AnnasBaseURL)
	}
}

type assertiveError string

func (e assertiveError) Error() string { return string(e) }
