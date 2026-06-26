package env

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/iosifache/annas-mcp/internal/logger"
	"github.com/iosifache/annas-mcp/internal/mirror"
	"go.uber.org/zap"
)

const DefaultAnnasBaseURL = "annas-archive.gl"

type Env struct {
	SecretKey    string `json:"secret"`
	DownloadPath string `json:"download_path"`
	AnnasBaseURL string `json:"annas_base_url"`
}

var (
	resolvedEnvOnce        sync.Once
	resolvedAnnasBaseURL   string
	resolveAnnasBaseURLErr error
	resolveAnnasBaseURL    = defaultResolveAnnasBaseURL
)

func GetAnnasBaseURL() string {
	l := logger.GetLogger()

	fallbackBaseURL := normalizeBaseURL(os.Getenv("ANNAS_BASE_URL"))
	if !autoMirrorDiscoveryEnabled() {
		if fallbackBaseURL != "" {
			return fallbackBaseURL
		}
		return DefaultAnnasBaseURL
	}

	resolvedEnvOnce.Do(func() {
		resolvedAnnasBaseURL, resolveAnnasBaseURLErr = resolveAnnasBaseURL()
	})

	if resolveAnnasBaseURLErr != nil {
		l.Warn("Automatic Anna mirror resolution failed, using configured mirror",
			zap.String("ANNAS_BASE_URL", fallbackBaseURL),
			zap.Error(resolveAnnasBaseURLErr),
		)
	}

	if resolved := normalizeBaseURL(resolvedAnnasBaseURL); resolved != "" {
		return resolved
	}

	if fallbackBaseURL != "" {
		return fallbackBaseURL
	}

	return DefaultAnnasBaseURL
}

func GetEnv() (*Env, error) {
	l := logger.GetLogger()

	secretKey := os.Getenv("ANNAS_SECRET_KEY")
	downloadPath := os.Getenv("ANNAS_DOWNLOAD_PATH")
	annasBaseURL := GetAnnasBaseURL()
	if secretKey == "" || downloadPath == "" {
		err := errors.New("ANNAS_SECRET_KEY and ANNAS_DOWNLOAD_PATH environment variables must be set")

		// Never log secret keys - use boolean flags instead
		l.Error("Environment variables not set",
			zap.Bool("ANNAS_SECRET_KEY_set", secretKey != ""),
			zap.String("ANNAS_DOWNLOAD_PATH", downloadPath),
			zap.String("ANNAS_BASE_URL", annasBaseURL),
			zap.Error(err),
		)

		return nil, err
	}

	if !filepath.IsAbs(downloadPath) {
		return nil, fmt.Errorf("ANNAS_DOWNLOAD_PATH must be an absolute path, got: %s", downloadPath)
	}

	return &Env{
		SecretKey:    secretKey,
		DownloadPath: downloadPath,
		AnnasBaseURL: annasBaseURL,
	}, nil
}

func defaultResolveAnnasBaseURL() (string, error) {
	fallbackBaseURL := normalizeBaseURL(os.Getenv("ANNAS_BASE_URL"))
	resolver := mirror.NewResolver(nil, mirror.DefaultStatusPageURL, nil)
	return resolver.Resolve(context.Background(), mirror.ResolveOptions{FallbackBaseURL: fallbackBaseURL})
}

func autoMirrorDiscoveryEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("ANNAS_AUTO_BASE_URL"))
	return err == nil && enabled
}

func normalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	return value
}

func resetResolvedEnvForTests() {
	resolvedEnvOnce = sync.Once{}
	resolvedAnnasBaseURL = ""
	resolveAnnasBaseURLErr = nil
}
