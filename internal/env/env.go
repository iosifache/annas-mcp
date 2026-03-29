package env

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

const DefaultAnnasBaseURL = "annas-archive.gl"

type Env struct {
	SecretKey    string `json:"secret"`
	DownloadPath string `json:"download_path"`
	AnnasBaseURL string `json:"annas_base_url"`
}

func GetEnv() (*Env, error) {
	l := logger.GetLogger()

	secretKey := os.Getenv("ANNAS_SECRET_KEY")
	downloadPath := os.Getenv("ANNAS_DOWNLOAD_PATH")
	annasBaseURL := os.Getenv("ANNAS_BASE_URL")

	if downloadPath != "" && !filepath.IsAbs(downloadPath) {
		return nil, fmt.Errorf("ANNAS_DOWNLOAD_PATH must be an absolute path, got: %s", downloadPath)
	}

	if annasBaseURL == "" {
		annasBaseURL = DefaultAnnasBaseURL
	}

	l.Debug("Environment loaded",
		zap.Bool("ANNAS_SECRET_KEY_set", secretKey != ""),
		zap.Bool("ANNAS_DOWNLOAD_PATH_set", downloadPath != ""),
		zap.String("ANNAS_BASE_URL", annasBaseURL),
	)

	return &Env{
		SecretKey:    secretKey,
		DownloadPath: downloadPath,
		AnnasBaseURL: annasBaseURL,
	}, nil
}
