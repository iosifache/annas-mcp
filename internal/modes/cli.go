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

	bookSearchCmd := &cobra.Command{
		Use:   "book-search [query]",
		Short: "Search for books by title, author, or topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			l.Info("Book search command called", zap.String("query", query))

			books, err := anna.FindBook(query)
			if err != nil {
				l.Error("Book search command failed",
					zap.String("query", query),
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

			l.Info("Book search command completed successfully",
				zap.String("query", query),
				zap.Int("resultsCount", len(books)),
			)

			return nil
		},
	}

	bookDownloadCmd := &cobra.Command{
		Use:   "book-download [hash] [filename]",
		Short: "Download a book by its MD5 hash",
		Long:  "Download a book by its MD5 hash to the specified filename. Requires ANNAS_SECRET_KEY and ANNAS_DOWNLOAD_PATH environment variables.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookHash := args[0]
			filename := args[1]

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
			if env.SecretKey == "" || env.DownloadPath == "" {
				return fmt.Errorf("ANNAS_SECRET_KEY and ANNAS_DOWNLOAD_PATH environment variables must be set for downloads")
			}

			book := &anna.Book{
				Hash:   bookHash,
				Title:  title,
				Format: format,
			}

			err = book.Download(env.SecretKey, env.DownloadPath)
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

	articleSearchCmd := &cobra.Command{
		Use:   "article-search [query]",
		Short: "Search for articles by DOI or keywords",
		Long:  "Search for academic articles. Provide a DOI (starting with '10.') or keywords.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			l.Info("Article search command called", zap.String("query", query))

			// Auto-detect if input is a DOI (starts with "10.")
			if strings.HasPrefix(strings.TrimSpace(query), "10.") {
				// DOI lookup
				l.Info("Detected DOI format, performing DOI lookup", zap.String("doi", query))

				paper, err := anna.LookupDOI(query)
				if err != nil {
					l.Error("DOI lookup failed",
						zap.String("doi", query),
						zap.Error(err),
					)
					return fmt.Errorf("DOI lookup failed: %w", err)
				}

				fmt.Println(paper.String())

				l.Info("DOI lookup completed", zap.String("doi", query))
				return nil
			}

			// Article keyword search
			l.Info("Performing article keyword search", zap.String("query", query))

			papers, err := anna.FindArticle(query)
			if err != nil {
				l.Error("Article search failed",
					zap.String("query", query),
					zap.Error(err),
				)
				return fmt.Errorf("article search failed: %w", err)
			}

			if len(papers) == 0 {
				fmt.Println("No articles found.")
				return nil
			}

			for i, paper := range papers {
				fmt.Printf("Article %d:\n%s\n", i+1, paper.String())
				if i < len(papers)-1 {
					fmt.Println()
				}
			}

			l.Info("Article search completed successfully",
				zap.String("query", query),
				zap.Int("resultsCount", len(papers)),
			)

			return nil
		},
	}

	articleDownloadCmd := &cobra.Command{
		Use:   "article-download [doi]",
		Short: "Download an article by its DOI",
		Long:  "Download an academic article/paper by providing its DOI.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			doi := args[0]
			l.Info("Article download command called", zap.String("doi", doi))

			env, err := env.GetEnv()
			if err != nil {
				l.Error("Failed to get environment variables", zap.Error(err))
				return fmt.Errorf("failed to get environment: %w", err)
			}
			if env.DownloadPath == "" {
				return fmt.Errorf("ANNAS_DOWNLOAD_PATH environment variable must be set for downloads")
			}

			// Lookup paper
			paper, err := anna.LookupDOI(doi)
			if err != nil {
				l.Error("DOI lookup failed for download",
					zap.String("doi", doi),
					zap.Error(err),
				)
				return fmt.Errorf("DOI lookup failed: %w", err)
			}

			// Try fast download first if hash and secret key available
			if paper.Hash != "" && env.SecretKey != "" {
				book := &anna.Book{
					Hash:   paper.Hash,
					Title:  paper.Title,
					Format: "pdf",
				}
				if err := book.Download(env.SecretKey, env.DownloadPath); err == nil {
					fmt.Printf("Article downloaded successfully to: %s\n", env.DownloadPath)
					l.Info("Article downloaded via fast download",
						zap.String("doi", doi),
						zap.String("path", env.DownloadPath),
					)
					return nil
				}
				l.Warn("Fast download failed, trying SciDB download",
					zap.String("doi", doi),
					zap.Error(err),
				)
			}

			// Fall back to SciDB download
			if err := paper.Download(env.DownloadPath); err != nil {
				l.Error("SciDB download failed",
					zap.String("doi", doi),
					zap.Error(err),
				)
				return fmt.Errorf("download failed: %w", err)
			}

			fmt.Printf("Article downloaded successfully to: %s\n", env.DownloadPath)
			l.Info("Article downloaded via SciDB",
				zap.String("doi", doi),
				zap.String("path", env.DownloadPath),
			)

			return nil
		},
	}

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

	rootCmd.AddCommand(bookSearchCmd)
	rootCmd.AddCommand(bookDownloadCmd)
	rootCmd.AddCommand(articleSearchCmd)
	rootCmd.AddCommand(articleDownloadCmd)
	rootCmd.AddCommand(mcpCmd)

	if err := fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithVersion(version.GetVersion()),
	); err != nil {
		os.Exit(1)
	}
}
