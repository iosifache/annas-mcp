package modes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/fang"
	"github.com/iosifache/annas-mcp/internal/anna"
	"github.com/iosifache/annas-mcp/internal/env"
	"github.com/iosifache/annas-mcp/internal/logger"
	"github.com/iosifache/annas-mcp/internal/version"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func StartCLI() {
	l := logger.GetLogger()
	defer l.Sync()

	if err := godotenv.Load(); err != nil {
		l.Warn("Error loading .env file", zap.Error(err))
	}

	rootCmd := &cobra.Command{
		Use:   "annas-mcp",
		Short: "Anna's Archive MCP CLI",
		Long:  "A command-line interface for searching and downloading books from Anna's Archive.",
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		Version: version.GetVersion(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	searchCmd := &cobra.Command{
		Use:   "search [term]",
		Short: "Search for books",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			searchTerm := args[0]
			l.Info("Search command called", zap.String("searchTerm", searchTerm))

			books, err := anna.FindBook(searchTerm)
			if err != nil {
				l.Error("Search command failed",
					zap.String("searchTerm", searchTerm),
					zap.Error(err),
				)
				return fmt.Errorf("failed to search books: %w", err)
			}

			if len(books) == 0 {
				fmt.Println("No books found.")
				return nil
			}

			for i, book := range books {
				fmt.Printf("Book %d:\n%s\n", i+1, book.String())
				if i < len(books)-1 {
					fmt.Println()
				}
			}

			l.Info("Search command completed successfully",
				zap.String("searchTerm", searchTerm),
				zap.Int("resultsCount", len(books)),
			)

			return nil
		},
	}

	downloadCmd := &cobra.Command{
		Use:   "download [hash] [filename]",
		Short: "Download a book by its MD5 hash",
		Long:  "Download a book by its MD5 hash to the specified filename. Requires ANNAS_SECRET_KEY and ANNAS_DOWNLOAD_PATH environment variables.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookHash := args[0]
			filename := args[1]

			pathIndex, err := cmd.Flags().GetInt("path-index")
			if err != nil {
				return fmt.Errorf("failed to read path-index flag: %w", err)
			}
			domainIndex, err := cmd.Flags().GetInt("domain-index")
			if err != nil {
				return fmt.Errorf("failed to read domain-index flag: %w", err)
			}

			ext := filepath.Ext(filename)
			if ext == "" {
				return fmt.Errorf("filename must include an extension (e.g., .pdf, .epub)")
			}
			format := strings.TrimPrefix(ext, ".")
			title := strings.TrimSuffix(filepath.Base(filename), ext)

			l.Info("Download command called",
				zap.String("bookHash", bookHash),
				zap.String("filename", filename),
				zap.String("title", title),
				zap.String("format", format),
			)

			env, err := env.GetEnv()
			if err != nil {
				l.Error("Failed to get environment variables", zap.Error(err))
				return fmt.Errorf("failed to get environment: %w", err)
			}

			book := &anna.Book{
				Hash:   bookHash,
				Title:  title,
				Format: format,
			}

			err = book.Download(env.SecretKey, env.DownloadPath, pathIndex, domainIndex)
			if err != nil {
				l.Error("Download command failed",
					zap.String("bookHash", bookHash),
					zap.String("downloadPath", env.DownloadPath),
					zap.Error(err),
				)
				return fmt.Errorf("failed to download book: %w", err)
			}

			fullPath := filepath.Join(env.DownloadPath, filename)
			fmt.Printf("Book downloaded successfully to: %s\n", fullPath)

			l.Info("Download command completed successfully",
				zap.String("bookHash", bookHash),
				zap.String("downloadPath", env.DownloadPath),
				zap.String("filename", filename),
			)

			return nil
		},
	}
	downloadCmd.Flags().Int("path-index", 0, "Collection path index for fast downloads")
	downloadCmd.Flags().Int("domain-index", 0, "Fast download server index")

	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP server",
		Long:  "Start the Model Context Protocol (MCP) server for integration with AI assistants.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Exit CLI mode and start MCP server
			StartMCPServer()
			return nil
		},
	}

	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(mcpCmd)

	if err := fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithVersion(version.GetVersion()),
	); err != nil {
		os.Exit(1)
	}
}
