package env

import (
	"errors"
	"os"

	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

// annas-archive.org was suspended in Jan 2026; .pm is currently working
const DefaultAnnasBaseURL = "annas-archive.pm"

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
	if secretKey == "" || downloadPath == "" {
		err := errors.New("ANNAS_SECRET_KEY and ANNAS_DOWNLOAD_PATH environment variables must be set")

		l.Error("Environment variables not set",
			zap.String("ANNAS_SECRET_KEY", secretKey),
			zap.String("ANNAS_DOWNLOAD_PATH", downloadPath),
			zap.String("ANNAS_BASE_URL", annasBaseURL),
			zap.Error(err),
		)

		return nil, err
	}

	if annasBaseURL == "" {
		annasBaseURL = DefaultAnnasBaseURL
	}

	return &Env{
		SecretKey:    secretKey,
		DownloadPath: downloadPath,
		AnnasBaseURL: annasBaseURL,
	}, nil
}
