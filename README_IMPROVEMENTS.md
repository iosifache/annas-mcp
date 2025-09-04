# annas-mcp Improvements

This fork includes significant improvements to the original annas-mcp tool, addressing IPv6 connectivity issues and enhancing search functionality.

## 🚀 Key Improvements

### 1. IPv6 Connectivity Fix
- **Problem**: Original tool fails with IPv6 socket errors on many systems
- **Solution**: Added IPv4-preferring HTTP client with proper fallback handling
- **Files Modified**: 
  - `internal/anna/anna.go` - Enhanced HTTP client with IPv4 preference
  - `Makefile` - Added build options for IPv6-challenged environments

### 2. Enhanced Search Functionality  
- **Problem**: Search results showed minimal information (just URLs)
- **Solution**: Implemented proper HTML parsing to extract book details
- **New Features**:
  - Full book titles extraction
  - File format detection (PDF, EPUB, MOBI, etc.)
  - File size information
  - Duplicate result filtering
  - JSON export of search results

### 3. Python Implementation (Fallback)
- **Problem**: Go build issues on systems with IPv6 problems
- **Solution**: Created a complete Python reimplementation
- **Features**:
  - Full feature parity with Go version
  - Better error handling and progress reporting
  - BeautifulSoup-based HTML parsing
  - IPv4-only networking

## 🛠 Technical Details

### IPv6 Fix Implementation
```go
// createIPv4PreferringClient creates an HTTP client that prefers IPv4 connections
func createIPv4PreferringClient() *http.Client {
    dialer := &net.Dialer{
        Timeout:   30 * time.Second,
        KeepAlive: 30 * time.Second,
    }
    
    transport := &http.Transport{
        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
            // First try IPv4
            if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
                return conn, nil
            }
            // Fallback to default (which includes IPv6)
            return dialer.DialContext(ctx, network, addr)
        },
        // ... additional transport configuration
    }
    
    return &http.Client{
        Transport: transport,
        Timeout:   60 * time.Second,
    }
}
```

### Search Enhancement
- Extracts book titles from `<a>` tags with MD5 hrefs
- Filters out empty image links
- Parses metadata from surrounding HTML elements
- Uses regex patterns to identify file formats and sizes
- Exports results to JSON for further processing

## 📊 Search Results Comparison

**Before:**
```
No books found.
```

**After:**
```
Found 50 books:
 1. Multiple Myeloma - A Medical Dictionary, Bibliography, and Annotated Research Guide to Internet References (PDF, 6.1MB) - 753c584f...
 2. Multiple Myeloma: Methods and Protocols (Methods in Molecular Medicine (113)) (PDF, 3.3MB) - 87fb8f19...
 ...
```

## 🔧 Build and Installation

### Standard Build (if Go networking works):
```bash
make build
make install
```

### Safe Build (with IPv6 issues):
```bash
make build-safe
make dev-install
```

### Python Fallback:
```bash
# Install dependencies
pip3 install requests beautifulsoup4 --user --break-system-packages

# Use Python version directly
python3 annas-mcp-improved.py search "your query"
python3 annas-mcp-improved.py download hash filename.pdf
```

## 🧪 Testing

Both search and download functionality have been thoroughly tested:

- **Search**: Returns detailed book information with titles, formats, and sizes
- **Download**: Maintains original API compatibility with improved error handling
- **IPv6 Fix**: Tested on systems with IPv6 connectivity issues

## 📁 Files Changed

- `internal/anna/anna.go` - IPv6 fix and enhanced search parsing
- `Makefile` - Build improvements
- `annas-mcp-improved.py` - Python fallback implementation
- `README_IMPROVEMENTS.md` - This documentation

## 🤝 Contributing

This fork maintains compatibility with the original annas-mcp while adding significant improvements. All changes are backward-compatible and can be merged upstream without breaking existing functionality.