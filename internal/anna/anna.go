package anna

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"strings"
	"sync"

	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	colly "github.com/gocolly/colly/v2"
	"github.com/iosifache/annas-mcp/internal/env"
	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

const (
	AnnasSearchEndpointFormat   = "https://%s/search?q=%s&content=%s"
	AnnasSciDBEndpointFormat    = "https://%s/scidb/%s"
	AnnasDownloadEndpointFormat = "https://%s/dyn/api/fast_download.json?md5=%s&key=%s"
	BrowserUserAgent            = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	// Regex to sanitize filenames - removes dangerous characters
	unsafeFilenameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
)

func extractMetaInformation(meta string) (language, format, size string) {
	// The meta format may be:
	// - "✅ English [en] · EPUB · 0.7MB · 2015 · ..."
	// - "✅ English [en] · Hindi [hi] · EPUB · 0.7MB · ..."
	parts := strings.Split(meta, " · ")
	if len(parts) < 3 {
		return "", "", ""
	}

	// Extract language from first part
	languagePart := strings.TrimSpace(parts[0])
	if idx := strings.Index(languagePart, "["); idx > 0 {
		language = strings.TrimSpace(languagePart[:idx])
		// Remove checkmark and leading spaces properly
		language = strings.TrimPrefix(language, "✅")
		language = strings.TrimSpace(language)
	}

	// Common ebook formats (case-insensitive search)
	formatRegex := regexp.MustCompile(`(?i)\b(EPUB|PDF|MOBI|AZW3|AZW|DJVU|CBZ|CBR|FB2|DOCX?|TXT)\b`)

	// Size indicators
	sizeRegex := regexp.MustCompile(`\d+\.?\d*\s*(MB|KB|GB|TB)`)

	// Search through parts for format and size
	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])

		// Check for size
		if size == "" && sizeRegex.MatchString(part) {
			size = part
		}

		// Check for format
		if format == "" && formatRegex.MatchString(part) {
			matches := formatRegex.FindStringSubmatch(part)
			if len(matches) > 0 {
				format = strings.ToUpper(matches[1])
			}
		}

		// Early exit if we found both
		if format != "" && size != "" {
			break
		}
	}

	return language, format, size
}

// sanitizeFilename removes dangerous characters and prevents path traversal
func sanitizeFilename(filename string) string {
	// Replace unsafe characters with underscores
	safe := unsafeFilenameChars.ReplaceAllString(filename, "_")

	// Remove any path separators and ".." sequences
	safe = strings.ReplaceAll(safe, "..", "_")
	safe = filepath.Base(safe)

	// Limit filename length (255 is typical max, leave room for extension)
	if len(safe) > 200 {
		safe = safe[:200]
	}

	return safe
}

func FindBook(query string) ([]*Book, error) {
	l := logger.GetLogger()

	// Use mutex to protect concurrent slice access
	var bookListMutex sync.Mutex
	bookList := make([]*colly.HTMLElement, 0)

	c := colly.NewCollector(
		colly.Async(true),
		// Set realistic User-Agent to avoid DDoS-Guard blocking
		colly.UserAgent(BrowserUserAgent),
	)

	c.OnHTML("a[href^='/md5/']", func(e *colly.HTMLElement) {
		// Only process the first link (the cover image link), not the duplicate title link
		if e.Attr("class") == "custom-a block mr-2 sm:mr-4 hover:opacity-80" {
			bookListMutex.Lock()
			bookList = append(bookList, e)
			bookListMutex.Unlock()
		}
	})

	c.OnRequest(func(r *colly.Request) {
		l.Info("Visiting URL", zap.String("url", r.URL.String()))
	})

	// Add error handler
	c.OnError(func(r *colly.Response, err error) {
		status := 0
		if r != nil {
			status = r.StatusCode
		}
		l.Error("Search request failed",
			zap.Int("statusCode", status),
			zap.Error(err),
		)
	})

	env, err := env.GetEnv()
	if err != nil {
		return nil, err
	}

	fullURL := fmt.Sprintf(AnnasSearchEndpointFormat, env.AnnasBaseURL, url.QueryEscape(query), "book_any")

	if err := c.Visit(fullURL); err != nil {
		l.Error("Failed to visit search URL", zap.String("url", fullURL), zap.Error(err))
		return nil, fmt.Errorf("failed to visit search URL: %w", err)
	}
	c.Wait()

	bookListParsed := make([]*Book, 0)
	for _, e := range bookList {
		// Validate that parent and container elements exist
		parent := e.DOM.Parent()
		if parent.Length() == 0 {
			l.Warn("Skipping book: no parent element found")
			continue
		}

		bookInfoDiv := parent.Find("div.max-w-full")
		if bookInfoDiv.Length() == 0 {
			l.Warn("Skipping book: book info container not found")
			continue
		}

		// Extract title
		titleElement := bookInfoDiv.Find("a[href^='/md5/']")
		title := strings.TrimSpace(titleElement.Text())
		if title == "" {
			l.Warn("Skipping book: title is empty")
			continue
		}

		// Extract authors (optional)
		authorsRaw := bookInfoDiv.Find("a[href^='/search'] span.icon-\\[mdi--user-edit\\]").Parent().Text()
		authors := strings.TrimSpace(authorsRaw)

		// Extract publisher (optional)
		publisherRaw := bookInfoDiv.Find("a[href^='/search'] span.icon-\\[mdi--company\\]").Parent().Text()
		publisher := strings.TrimSpace(publisherRaw)

		// Extract metadata
		meta := bookInfoDiv.Find("div.text-gray-800").Text()
		language, format, size := extractMetaInformation(meta)

		// Extract link and hash
		link := e.Attr("href")
		if link == "" {
			l.Warn("Skipping book: no link found", zap.String("title", title))
			continue
		}
		hash := strings.TrimPrefix(link, "/md5/")
		if hash == "" {
			l.Warn("Skipping book: no hash found", zap.String("title", title))
			continue
		}

		book := &Book{
			Language:  language,
			Format:    format,
			Size:      size,
			Title:     title,
			Publisher: publisher,
			Authors:   authors,
			URL:       e.Request.AbsoluteURL(link),
			Hash:      hash,
		}

		bookListParsed = append(bookListParsed, book)
	}

	// Log result count for debugging
	l.Info("Search completed",
		zap.Int("totalElements", len(bookList)),
		zap.Int("validBooks", len(bookListParsed)),
	)

	return bookListParsed, nil
}

func FindArticle(query string) ([]*Paper, error) {
	l := logger.GetLogger()

	// Use mutex to protect concurrent slice access
	var paperListMutex sync.Mutex
	paperList := make([]*colly.HTMLElement, 0)

	c := colly.NewCollector(
		colly.Async(true),
		// Set realistic User-Agent to avoid DDoS-Guard blocking
		colly.UserAgent(BrowserUserAgent),
	)

	c.OnHTML("a[href^='/md5/']", func(e *colly.HTMLElement) {
		// Only process the first link (the cover image link), not the duplicate title link
		if e.Attr("class") == "custom-a block mr-2 sm:mr-4 hover:opacity-80" {
			paperListMutex.Lock()
			paperList = append(paperList, e)
			paperListMutex.Unlock()
		}
	})

	c.OnRequest(func(r *colly.Request) {
		l.Info("Visiting URL", zap.String("url", r.URL.String()))
	})

	// Add error handler
	c.OnError(func(r *colly.Response, err error) {
		status := 0
		if r != nil {
			status = r.StatusCode
		}
		l.Error("Article search request failed",
			zap.Int("statusCode", status),
			zap.Error(err),
		)
	})

	env, err := env.GetEnv()
	if err != nil {
		return nil, err
	}

	fullURL := fmt.Sprintf(AnnasSearchEndpointFormat, env.AnnasBaseURL, url.QueryEscape(query), "journal")

	if err := c.Visit(fullURL); err != nil {
		l.Error("Failed to visit article search URL", zap.String("url", fullURL), zap.Error(err))
		return nil, fmt.Errorf("failed to visit article search URL: %w", err)
	}
	c.Wait()

	paperListParsed := make([]*Paper, 0)
	for _, e := range paperList {
		// Validate that parent and container elements exist
		parent := e.DOM.Parent()
		if parent.Length() == 0 {
			l.Warn("Skipping article: no parent element found")
			continue
		}

		paperInfoDiv := parent.Find("div.max-w-full")
		if paperInfoDiv.Length() == 0 {
			l.Warn("Skipping article: info container not found")
			continue
		}

		// Extract title
		titleElement := paperInfoDiv.Find("a[href^='/md5/']")
		title := strings.TrimSpace(titleElement.Text())
		if title == "" {
			l.Warn("Skipping article: title is empty")
			continue
		}

		// Extract authors (optional)
		authorsRaw := paperInfoDiv.Find("a[href^='/search'] span.icon-\\[mdi--user-edit\\]").Parent().Text()
		authors := strings.TrimSpace(authorsRaw)

		// Extract journal/publisher (optional)
		journalRaw := paperInfoDiv.Find("a[href^='/search'] span.icon-\\[mdi--company\\]").Parent().Text()
		journal := strings.TrimSpace(journalRaw)

		// Extract metadata
		meta := paperInfoDiv.Find("div.text-gray-800").Text()
		_, _, size := extractMetaInformation(meta)

		// Extract link and hash
		link := e.Attr("href")
		if link == "" {
			l.Warn("Skipping article: no link found", zap.String("title", title))
			continue
		}
		hash := strings.TrimPrefix(link, "/md5/")
		if hash == "" {
			l.Warn("Skipping article: no hash found", zap.String("title", title))
			continue
		}

		paper := &Paper{
			Title:     title,
			Authors:   authors,
			Journal:   journal,
			Size:      size,
			Hash:      hash,
			PageURL:   e.Request.AbsoluteURL(link),
		}

		paperListParsed = append(paperListParsed, paper)
	}

	// Log result count for debugging
	l.Info("Article search completed",
		zap.Int("totalElements", len(paperList)),
		zap.Int("validPapers", len(paperListParsed)),
	)

	return paperListParsed, nil
}

func (b *Book) Download(secretKey, folderPath string) error {
	l := logger.GetLogger()

	env, err := env.GetEnv()
	if err != nil {
		return fmt.Errorf("failed to get environment: %w", err)
	}

	// First API call: get the signed download URL. Small JSON
	// response; a whole-request timeout is appropriate here.
	apiURL := fmt.Sprintf(AnnasDownloadEndpointFormat, env.AnnasBaseURL, b.Hash, secretKey)
	l.Info("Fetching download URL", zap.String("hash", b.Hash))

	apiClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := apiClient.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch download URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, resp.Status)
		}
		return fmt.Errorf("API request failed with status %d: %s (body: %s)",
			resp.StatusCode, resp.Status, string(body))
	}

	var apiResp fastDownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to decode API response: %w", err)
	}
	if apiResp.DownloadURL == "" {
		if apiResp.Error != "" {
			return fmt.Errorf("API error: %s", apiResp.Error)
		}
		return errors.New("API returned empty download URL")
	}

	// Second call: stream the file body with stall-based deadlines.
	// There is no upper bound on transfer duration — only on periods
	// of zero progress — so large books over slow links succeed as
	// long as bytes keep flowing.
	l.Info("Downloading file", zap.String("url", apiResp.DownloadURL))
	ctx, downloadResp, body, cleanup, err := stallingGet(
		context.Background(), apiResp.DownloadURL, BrowserUserAgent,
		30*time.Second, 60*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer cleanup()
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d: %s", downloadResp.StatusCode, downloadResp.Status)
	}

	safeTitle := sanitizeFilename(b.Title)
	if safeTitle == "" {
		safeTitle = "untitled"
	}
	format := strings.ToLower(b.Format)
	if format == "" {
		format = "bin"
	}
	filePath := filepath.Join(folderPath, safeTitle+"."+format)

	l.Info("Creating file", zap.String("path", filePath))
	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	success := false
	defer func() {
		out.Close()
		if !success {
			if removeErr := os.Remove(filePath); removeErr != nil {
				l.Warn("Failed to remove partial file",
					zap.String("path", filePath),
					zap.Error(removeErr),
				)
			}
		}
	}()

	written, err := io.Copy(out, body)
	if err != nil {
		if errors.Is(context.Cause(ctx), ErrStalled) {
			return fmt.Errorf("download stalled after %d bytes: %w", written, ErrStalled)
		}
		return fmt.Errorf("failed to write file (wrote %d bytes): %w", written, err)
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}

	success = true
	l.Info("Download completed successfully",
		zap.String("path", filePath),
		zap.Int64("bytes", written),
	)
	return nil
}

func LookupDOI(doi string) (*Paper, error) {
	l := logger.GetLogger()

	env, err := env.GetEnv()
	if err != nil {
		return nil, err
	}

	paper := &Paper{DOI: doi}

	// Phase 1: Visit /scidb/DOI which redirects to a search results page.
	// Extract the MD5 hash from the first search result.
	searchCollector := colly.NewCollector(
		colly.UserAgent(BrowserUserAgent),
	)

	searchCollector.OnHTML("a[href^='/md5/']", func(e *colly.HTMLElement) {
		if paper.Hash != "" {
			return
		}
		href := e.Attr("href")
		hash := strings.TrimPrefix(href, "/md5/")
		if hash != "" {
			paper.Hash = hash
		}
	})

	searchCollector.OnError(func(r *colly.Response, err error) {
		status := 0
		if r != nil {
			status = r.StatusCode
		}
		l.Error("SciDB search failed",
			zap.String("doi", doi),
			zap.Int("statusCode", status),
			zap.Error(err),
		)
	})

	scidbURL := fmt.Sprintf(AnnasSciDBEndpointFormat, env.AnnasBaseURL, doi)
	paper.PageURL = scidbURL

	l.Info("Looking up DOI", zap.String("url", scidbURL))

	if err := searchCollector.Visit(scidbURL); err != nil {
		return nil, fmt.Errorf("failed to lookup DOI: %w", err)
	}

	if paper.Hash == "" {
		return nil, fmt.Errorf("no paper found for DOI: %s", doi)
	}

	// Phase 2: Visit /md5/HASH to get paper details.
	detailCollector := colly.NewCollector(
		colly.UserAgent(BrowserUserAgent),
	)

	detailCollector.OnHTML("title", func(e *colly.HTMLElement) {
		title := e.Text
		if idx := strings.Index(title, " - Anna"); idx > 0 {
			paper.Title = strings.TrimSpace(title[:idx])
		}
	})

	detailCollector.OnHTML("meta[name=description]", func(e *colly.HTMLElement) {
		// Format: "Authors\n\nPublisher (ISSN)\n\nJournal, #issue, vol, pages, year"
		desc := e.Attr("content")
		parts := strings.Split(desc, "\n\n")
		if len(parts) >= 3 {
			paper.Journal = strings.TrimSpace(parts[2])
		} else if len(parts) >= 2 {
			paper.Journal = strings.TrimSpace(parts[1])
		} else {
			paper.Journal = strings.TrimSpace(desc)
		}
	})

	// Extract authors from the detail page
	detailCollector.OnHTML("a[href^='/search']", func(e *colly.HTMLElement) {
		if paper.Authors != "" {
			return
		}
		// Author links contain a span with icon-[mdi--user-edit]
		if e.DOM.Find("span.icon-\\[mdi--user-edit\\]").Length() > 0 {
			paper.Authors = strings.TrimSpace(e.Text)
		}
	})

	// Extract size from metadata line
	detailCollector.OnHTML("div.text-gray-500", func(e *colly.HTMLElement) {
		text := e.Text
		if strings.Contains(text, "MB") || strings.Contains(text, "KB") {
			paper.Size = strings.TrimSpace(text)
		}
	})

	detailCollector.OnError(func(r *colly.Response, err error) {
		l.Warn("Failed to fetch paper details", zap.String("hash", paper.Hash), zap.Error(err))
	})

	md5URL := fmt.Sprintf("https://%s/md5/%s", env.AnnasBaseURL, paper.Hash)
	l.Info("Fetching paper details", zap.String("url", md5URL))

	if err := detailCollector.Visit(md5URL); err != nil {
		l.Warn("Failed to visit paper detail page", zap.Error(err))
		// Non-fatal: we still have the hash for downloading
	}

	// Set download URL for scidb (no browser verification required)
	paper.DownloadURL = fmt.Sprintf("/scidb?doi=%s", doi)

	return paper, nil
}

func (p *Paper) Download(folderPath string) error {
	l := logger.GetLogger()

	if p.DownloadURL == "" {
		return errors.New("no download URL available for this paper")
	}

	env, err := env.GetEnv()
	if err != nil {
		return fmt.Errorf("failed to get environment: %w", err)
	}

	downloadURL := p.DownloadURL
	if !strings.HasPrefix(downloadURL, "http") {
		downloadURL = fmt.Sprintf("https://%s%s", env.AnnasBaseURL, downloadURL)
	}

	l.Info("Downloading paper via SciDB", zap.String("url", downloadURL))
	ctx, resp, body, cleanup, err := stallingGet(
		context.Background(), downloadURL, BrowserUserAgent,
		30*time.Second, 60*time.Second,
	)
	if err != nil {
		return fmt.Errorf("failed to download paper: %w", err)
	}
	defer cleanup()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, resp.Status)
		}
		return fmt.Errorf("download failed with status %d: %s (body: %s)", resp.StatusCode, resp.Status, string(b))
	}

	// Infer file extension from Content-Disposition or Content-Type
	ext := ".pdf"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn, ok := params["filename"]; ok {
				if e := filepath.Ext(fn); e != "" {
					ext = e
				}
			}
		}
	} else if ct := resp.Header.Get("Content-Type"); ct != "" {
		exts, _ := mime.ExtensionsByType(ct)
		if len(exts) > 0 {
			ext = exts[0]
		}
	}

	name := p.Title
	if name == "" {
		name = p.DOI
	}
	safeName := sanitizeFilename(name)
	if safeName == "" {
		safeName = "paper"
	}
	filePath := filepath.Join(folderPath, safeName+ext)

	l.Info("Creating file", zap.String("path", filePath))
	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	success := false
	defer func() {
		out.Close()
		if !success {
			if removeErr := os.Remove(filePath); removeErr != nil {
				l.Warn("Failed to remove partial file",
					zap.String("path", filePath),
					zap.Error(removeErr),
				)
			}
		}
	}()

	written, copyErr := io.Copy(out, body)
	if copyErr != nil {
		if errors.Is(context.Cause(ctx), ErrStalled) {
			return fmt.Errorf("paper download stalled after %d bytes: %w", written, ErrStalled)
		}
		return fmt.Errorf("failed to write file (wrote %d bytes): %w", written, copyErr)
	}

	if err := out.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}

	success = true
	l.Info("Paper download completed successfully",
		zap.String("path", filePath),
		zap.Int64("bytes", written),
	)
	return nil
}

func (b *Book) String() string {
	return fmt.Sprintf("Title: %s\nAuthors: %s\nPublisher: %s\nLanguage: %s\nFormat: %s\nSize: %s\nURL: %s\nHash: %s",
		b.Title, b.Authors, b.Publisher, b.Language, b.Format, b.Size, b.URL, b.Hash)
}

func (b *Book) ToJSON() (string, error) {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}
