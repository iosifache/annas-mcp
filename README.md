# Anna's Archive MCP Server (and CLI Tool)

[An MCP server](https://modelcontextprotocol.io/introduction) and CLI tool for searching and downloading documents from [Anna's Archive](https://annas-archive.org)

## 🚀 Recent Improvements

This fork includes significant enhancements to address connectivity issues and improve functionality:

### ✅ **IPv6 Connectivity Fix**
- **Fixed**: IPv6 socket connection failures that prevented the tool from working on many systems
- **Solution**: Added IPv4-preferring HTTP client with graceful IPv6 fallback
- **Impact**: Tool now works reliably on all network configurations

### ✅ **Enhanced Search Functionality**
- **Before**: Search returned minimal information (just URLs)
- **After**: Rich search results with detailed book information
- **New Features**:
  - 📚 Full book title extraction
  - 📄 File format detection (PDF, EPUB, MOBI, TXT, etc.)
  - 📊 File size information (MB/KB/GB)
  - 🚫 Duplicate result filtering
  - 💾 JSON export of search results

### ✅ **Build & Compatibility Improvements**
- Added comprehensive Makefile with IPv6-safe build options
- Created Python fallback implementation with full feature parity
- Enhanced error handling and progress reporting
- Maintained 100% backward compatibility

### 📊 **Before/After Comparison**

**Before (Broken):**
```bash
$ annas-mcp search "python programming"
ERROR: write tcp [IPv6]:port->[IPv6]:443: write: socket is not connected
```

**After (Working):**
```bash
$ annas-mcp search "python programming"
Found 50 books:
 1. Python Programming for Beginners : The Ultimate Guide... (EPUB, 12.0MB) - f8744872...
 2. Python Programming: Python Programming for Beginners... (EPUB, 0.6MB) - 075d2675...
 3. Python Programming Basics: For Freshers Learn Python... (EPUB, 0.3MB) - 1d3c91aa...
 ...

Detailed results saved to: search_results.json
```

---

> [!NOTE]
> Notwithstanding prevailing public sentiment regarding Anna's Archive, the platform serves as a comprehensive repository for automated retrieval of documents released under permissive licensing frameworks (including Creative Commons publications and public domain materials). This software does not endorse unauthorized acquisition of copyrighted content and should be regarded solely as a utility. Users are urged to respect the intellectual property rights of authors and acknowledge the considerable effort invested in document creation.

## Available Operations

| Operation                                                                      | MCP Tool   | CLI Command |
| ------------------------------------------------------------------------------ | ---------- | ----------- |
| Search Anna's Archive for documents matching specified terms                   | `search`   | `search`    |
| Download a specific document that was previously returned by the `search` tool | `download` | `download`  |

## Requirements

If you plan to use only the CLI tool, you need:

- [A donation to Anna's Archive](https://annas-archive.org/donate), which grants JSON API access
- [An API key](https://annas-archive.org/faq#api)

If using the project as an MCP server, you also need an MCP client, such as [Claude Desktop](https://claude.ai/download).

The environment should contain two variables:

- `ANNAS_SECRET_KEY`: The API key
- `ANNAS_DOWNLOAD_PATH`: The path where the documents should be downloaded

## Setup

### Installation Options

#### Option 1: Improved Fork (Recommended)
This fork with IPv6 fixes and enhanced search functionality:

```bash
# Clone the improved fork
git clone https://github.com/trytofly94/annas-mcp.git
cd annas-mcp

# Build with improvements
make build
make install

# Or use safe build if you have IPv6 issues
make build-safe
make dev-install
```

#### Option 2: Python Fallback (If Go build fails)
If the Go build fails due to network issues, use the Python implementation:

```bash
# Install Python dependencies
pip3 install requests beautifulsoup4 --user --break-system-packages

# Use directly
python3 annas-mcp-improved.py search "your query"
python3 annas-mcp-improved.py download hash filename.pdf

# Or install as replacement
cp annas-mcp-improved.py ~/.local/bin/annas-mcp
chmod +x ~/.local/bin/annas-mcp
```

#### Option 3: Original Binary (May have IPv6 issues)
Download the appropriate binary from [the GitHub Releases section](https://github.com/iosifache/annas-mcp/releases).

If you plan to use the tool for its MCP server functionality, you need to integrate it into your MCP client. If you are using Claude Desktop, please consider the following example configuration:

```json
"anna-mcp": {
    "command": "/Users/iosifache/Downloads/annas-mcp",
    "args": ["mcp"],
    "env": {
        "ANNAS_SECRET_KEY": "feedfacecafebeef",
        "ANNAS_DOWNLOAD_PATH": "/Users/iosifache/Downloads"
    }
}
```

## Demo

### As an MCP Server

<img src="screenshots/claude.png" width="600px"/>

### As a CLI Tool

<img src="screenshots/cli.png" width="400px"/>

## Troubleshooting

### IPv6 Connection Issues
If you encounter errors like:
```
write tcp [IPv6]:port->[IPv6]:443: write: socket is not connected
```

**Solutions:**
1. Use the improved fork (Option 1 above) which includes the IPv6 fix
2. Use the Python fallback implementation (Option 2 above)
3. Temporarily force IPv4 resolution:
   ```bash
   sudo bash -c 'echo "188.114.97.3 annas-archive.org" >> /etc/hosts'
   # Use annas-mcp normally
   sudo sed -i '' '/188.114.97.3.*annas-archive.org/d' /etc/hosts  # Clean up afterward
   ```

### Search Returns "No books found"
If search returns no results but Anna's Archive has the content:
1. Ensure you're using the enhanced search functionality (this fork)
2. Try different search terms or more specific queries
3. Check that your API key is valid and has remaining quota

### Build Issues
If Go build fails with network errors:
1. Try `make build-safe` instead of `make build`
2. Use the Python implementation as a fallback
3. Set `GODEBUG=netdns=cgo+4` to force IPv4 DNS resolution

### Environment Variables Not Set
Make sure to export the required environment variables:
```bash
export ANNAS_SECRET_KEY="your-api-key-here"
export ANNAS_DOWNLOAD_PATH="/path/to/downloads"
```

Or create a configuration script:
```bash
# Save as ~/annas-mcp-env.sh
export ANNAS_SECRET_KEY="your-api-key-here"  
export ANNAS_DOWNLOAD_PATH="/path/to/downloads"

# Then source it before using
source ~/annas-mcp-env.sh
annas-mcp search "your query"
```
