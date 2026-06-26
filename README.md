# Anna's Archive MCP Server (and CLI Tool)

[An MCP server](https://modelcontextprotocol.io/introduction) and CLI tool for searching and downloading documents from [Anna's Archive](https://annas-archive.li), with optional automatic selection of a mirror reported as healthy by [SLUM](https://open-slum.org/).

> [!NOTE]
> Notwithstanding prevailing public sentiment regarding Anna's Archive, the platform serves as a comprehensive repository for automated retrieval of documents released under permissive licensing frameworks (including Creative Commons publications and public domain materials). This software does not endorse unauthorized acquisition of copyrighted content and should be regarded solely as a utility. Users are urged to respect the intellectual property rights of authors and acknowledge the considerable effort invested in document creation.

> [!WARNING]
> Please refer to [this section](#annas-archive-mirrors) if any of the links lead to a non-functional Anna's Archive server.

## Available Operations

| Operation                                      | MCP Tool           | CLI Command         | Example                                                      |
| ---------------------------------------------- | ------------------ | ------------------- | ------------------------------------------------------------ |
| Search for books by title, author, or topic   | `book_search`      | `book-search`       | `book-search "machine learning python"`                     |
| Download a book by its MD5 hash                | `book_download`    | `book-download`     | `book-download abc123def456 "my-book.pdf"`                  |
| Search for articles by DOI or keywords        | `article_search`   | `article-search`    | `article-search "10.1038/nature12345"` or `article-search "neural networks"` |
| Download an article by its DOI                 | `article_download` | `article-download`  | `article-download "10.1038/nature12345"`                    |

## Requirements

Search works without any required environment variables.

Downloads require:

- [A donation to Anna's Archive](https://annas-archive.li/donate), which grants JSON API access
- [An API key](https://annas-archive.li/faq#api)
- `ANNAS_SECRET_KEY`: The Anna's Archive API key.
- `ANNAS_DOWNLOAD_PATH`: The path where the documents should be downloaded.

If using the project as an MCP server, you also need an MCP client, such as [Claude Desktop](https://claude.ai/download).

Optionally, you can set:

- `ANNAS_BASE_URL`: The Anna mirror to use (defaults to `annas-archive.li`). When automatic mirror discovery is enabled, this becomes the fallback mirror.
- `ANNAS_AUTO_BASE_URL`: Set to `true` to discover the best available Anna mirror automatically from [SLUM](https://open-slum.org/).

These variables can also be stored in an `.env` file in the folder containing the binary.

By default, the tool uses `ANNAS_BASE_URL`, or the built-in default mirror when `ANNAS_BASE_URL` is not set. Automatic discovery is opt-in: when `ANNAS_AUTO_BASE_URL=true`, the tool reads the public status page, ranks discovered Anna mirror candidates by recent health and latency, probes them locally, and uses the best reachable mirror. If discovery or probing fails, the tool falls back to `ANNAS_BASE_URL`, then to the built-in default mirror.

HTTP requests default to a 1 hour timeout. For CLI usage, override this with `--timeout`, for example:

```bash
annas-mcp --timeout 1h book-download abc123def456 "my-book.pdf"
```

For MCP usage, tools accept an optional `timeout_seconds` parameter, for example `3600` for 1 hour.

## Setup

Download the appropriate binary from [the GitHub Releases section](https://github.com/iosifache/annas-mcp/releases).

If you plan to use the tool for its MCP server functionality, you need to integrate it into your MCP client. If you are using Claude Desktop, please consider the following example configuration:

```json
"anna-mcp": {
    "command": "/Users/iosifache/Downloads/annas-mcp",
    "args": ["mcp"],
    "env": {
        "ANNAS_SECRET_KEY": "feedfacecafebeef",
        "ANNAS_DOWNLOAD_PATH": "/Users/iosifache/Downloads",
        "ANNAS_BASE_URL": "annas-archive.li"
    }
}
```

## Demo

### As an MCP Server

<img src="screenshots/claude.png" width="600px"/>

### As a CLI Tool

<img src="screenshots/cli.png" width="400px"/>

## Anna's Archive Mirrors

Anna's Archive has multiple mirrors, and their availability can change over time. By default, this project uses `ANNAS_BASE_URL`, or `annas-archive.li` when `ANNAS_BASE_URL` is not set.

If you want the tool to choose a mirror automatically, set `ANNAS_AUTO_BASE_URL=true`. In this mode, the tool checks [SLUM](https://open-slum.org/), dynamically discovers Anna mirror candidates from the public status page, ranks them by recent health and latency, and confirms reachability locally before using one. Set `ANNAS_BASE_URL` as a fallback for environments where automatic discovery may fail.
