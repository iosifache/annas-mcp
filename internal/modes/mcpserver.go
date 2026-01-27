package modes

import (
	"context"

	"github.com/iosifache/annas-mcp/internal/anna"
	"github.com/iosifache/annas-mcp/internal/env"
	"github.com/iosifache/annas-mcp/internal/logger"
	"github.com/iosifache/annas-mcp/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

func SearchTool(ctx context.Context, cc *mcp.ServerSession, params *mcp.CallToolParamsFor[SearchParams]) (*mcp.CallToolResultFor[any], error) {
	l := logger.GetLogger()

	l.Info("Search command called",
		zap.String("searchTerm", params.Arguments.SearchTerm),
	)

	books, err := anna.FindBook(params.Arguments.SearchTerm)
	if err != nil {
		l.Error("Search command failed",
			zap.String("searchTerm", params.Arguments.SearchTerm),
			zap.Error(err),
		)
		return nil, err
	}

	bookList := ""
	for _, book := range books {
		bookList += book.String() + "\n\n"
	}

	l.Info("Search command completed successfully",
		zap.String("searchTerm", params.Arguments.SearchTerm),
		zap.Int("resultsCount", len(books)),
	)

	return &mcp.CallToolResultFor[any]{
		Content:           []mcp.Content{&mcp.TextContent{Text: bookList}},
		StructuredContent: books,
	}, nil
}

func DownloadTool(ctx context.Context, cc *mcp.ServerSession, params *mcp.CallToolParamsFor[DownloadParams]) (*mcp.CallToolResultFor[any], error) {
	l := logger.GetLogger()

	l.Info("Download command called",
		zap.String("bookHash", params.Arguments.BookHash),
		zap.String("title", params.Arguments.Title),
		zap.String("format", params.Arguments.Format),
	)

	env, err := env.GetEnv()
	if err != nil {
		l.Error("Failed to get environment variables", zap.Error(err))
		return nil, err
	}
	secretKey := env.SecretKey
	downloadPath := env.DownloadPath

	title := params.Arguments.Title
	format := params.Arguments.Format
	book := &anna.Book{
		Hash:   params.Arguments.BookHash,
		Title:  title,
		Format: format,
	}

	err = book.Download(secretKey, downloadPath, 0, 0)
	if err != nil {
		l.Error("Download command failed",
			zap.String("bookHash", params.Arguments.BookHash),
			zap.String("downloadPath", downloadPath),
			zap.Error(err),
		)
		return nil, err
	}

	l.Info("Download command completed successfully",
		zap.String("bookHash", params.Arguments.BookHash),
		zap.String("downloadPath", downloadPath),
	)

	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{&mcp.TextContent{
			Text: "Book downloaded successfully to path: " + downloadPath,
		}},
	}, nil
}

func StartMCPServer() {
	l := logger.GetLogger()
	defer l.Sync()

	serverVersion := version.GetVersion()
	l.Info("Starting MCP server",
		zap.String("name", "annas-mcp"),
		zap.String("version", serverVersion),
	)

	server := mcp.NewServer("annas-mcp", serverVersion, nil)

	server.AddTools(
		mcp.NewServerTool("search", "Search books", SearchTool, mcp.Input(
			mcp.Property("term", mcp.Description("Term to search for")),
		)),
		mcp.NewServerTool("download", "Download a book by its MD5 hash. Requires ANNAS_SECRET_KEY and ANNAS_DOWNLOAD_PATH environment variables.", DownloadTool, mcp.Input(
			mcp.Property("hash", mcp.Description("MD5 hash of the book to download")),
			mcp.Property("title", mcp.Description("Book title, used for filename")),
			mcp.Property("format", mcp.Description("Book format, for example pdf or epub")),
		)),
	)

	l.Info("MCP server started successfully")

	if err := server.Run(context.Background(), mcp.NewStdioTransport()); err != nil {
		l.Fatal("MCP server failed", zap.Error(err))
	}
}
