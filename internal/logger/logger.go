package logger

import (
	"log"
	"os"

	"go.uber.org/zap"
)

var logger *zap.Logger

func init() {
	var err error

	// Check if we're running the MCP server
	isMCPMode := false
	for _, arg := range os.Args[1:] {
		if arg == "mcp" {
			isMCPMode = true
			break
		}
	}

	if isMCPMode {
		logger, err = zap.NewProduction()
	} else {
		config := zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
		logger, err = config.Build()
	}
	
	if err != nil {
		log.Fatalf("Failed to initialize zap logger: %v", err)
	}
}

func GetLogger() *zap.Logger {
	return logger
}
