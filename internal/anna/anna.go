package anna

import (
	"fmt"
	"net/url"

	"strings"

	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	colly "github.com/gocolly/colly/v2"
	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

const (
	AnnasSearchEndpoint   = "https://annas-archive.org/search?q=%s"
	AnnasDownloadEndpoint = "https://annas-archive.org/dyn/api/fast_download.json?md5=%s&key=%s"
)

// createIPv4PreferringClient creates an HTTP client that prefers IPv4 connections
// to avoid IPv6 connectivity issues
func createIPv4PreferringClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		// Prefer IPv4 by setting a custom dialer that tries IPv4 first
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
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}

// httpClient is a shared IPv4-preferring HTTP client instance
var httpClient = createIPv4PreferringClient()

func extractMetaInformation(meta string) (language, format, size string) {
	parts := strings.Split(meta, ", ")
	if len(parts) < 5 {
		return "", "", ""
	}

	language = parts[0]
	format = parts[1]
	size = parts[3]

	return language, format, size
}

func FindBook(query string) ([]*Book, error) {
	l := logger.GetLogger()

	c := colly.NewCollector(
		colly.Async(true),
	)

	// Set the HTTP client for colly to use IPv4-preferring client
	c.SetClient(httpClient)

	bookList := make([]*colly.HTMLElement, 0)
	books := make([]*Book, 0)

	// Improved book detection - look for both MD5 links and book containers
	c.OnHTML("a[href^='/md5/']", func(e *colly.HTMLElement) {
		bookList = append(bookList, e)
	})

	// Parse book information from the search results
	c.OnHTML("div.js-vim-focus", func(e *colly.HTMLElement) {
		// Extract book details from search result div
		titleElement := e.DOM.Find("h3 a").First()
		metaElement := e.DOM.Find(".text-sm.text-gray-500").First()
		
		if titleElement.Length() > 0 {
			href, exists := titleElement.Attr("href")
			if exists && strings.HasPrefix(href, "/md5/") {
				// Extract MD5 from href: /md5/abcd1234.../filename
				parts := strings.Split(strings.TrimPrefix(href, "/md5/"), "/")
				if len(parts) > 0 {
					md5Hash := parts[0]
					title := titleElement.Text()
					meta := metaElement.Text()
					language, format, size := extractMetaInformation(meta)
					
					book := &Book{
						Title:    strings.TrimSpace(title),
						Hash:     md5Hash,
						Language: language,
						Format:   format,
						Size:     size,
						URL:      fmt.Sprintf("https://annas-archive.org%s", href),
					}
					books = append(books, book)
					l.Info("Found book", zap.String("title", book.Title), zap.String("hash", book.Hash))
				}
			}
		}
	})

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", "annas-mcp/1.0")
		l.Info("Visiting URL", zap.String("url", r.URL.String()))
	})

	c.OnError(func(r *colly.Response, err error) {
		l.Error("Scraping error", zap.Error(err), zap.String("url", r.Request.URL.String()))
	})

	fullURL := fmt.Sprintf(AnnasSearchEndpoint, url.QueryEscape(query))
	l.Info("Searching for books", zap.String("query", query), zap.String("url", fullURL))
	
	err := c.Visit(fullURL)
	if err != nil {
		l.Error("Failed to visit search URL", zap.Error(err))
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	
	c.Wait()

	// If modern parsing didn't work, try legacy method
	if len(books) == 0 && len(bookList) > 0 {
		l.Info("Falling back to legacy parsing method")
		for _, element := range bookList {
			href := element.Attr("href")
			if strings.HasPrefix(href, "/md5/") {
				parts := strings.Split(strings.TrimPrefix(href, "/md5/"), "/")
				if len(parts) > 0 {
					book := &Book{
						Title: element.Text,
						Hash:  parts[0],
					}
					books = append(books, book)
				}
			}
		}
	}

	bookListParsed := make([]*Book, 0)
	for _, e := range bookList {
		meta := e.DOM.Parent().Find("div.relative.top-\\[-1\\].pl-4.grow.overflow-hidden > div").Eq(0).Text()
		title := e.DOM.Parent().Find("div.relative.top-\\[-1\\].pl-4.grow.overflow-hidden > h3").Text()
		publisher := e.DOM.Parent().Find("div.relative.top-\\[-1\\].pl-4.grow.overflow-hidden > div").Eq(1).Text()
		authors := e.DOM.Parent().Find("div.relative.top-\\[-1\\].pl-4.grow.overflow-hidden > div").Eq(2).Text()

		language, format, size := extractMetaInformation(meta)

		link := e.Attr("href")
		hash := strings.TrimPrefix(link, "/md5/")

		book := &Book{
			Language:  strings.TrimSpace(language),
			Format:    strings.TrimSpace(format)[1:],
			Size:      strings.TrimSpace(size),
			Title:     strings.TrimSpace(title),
			Publisher: strings.TrimSpace(publisher),
			Authors:   strings.TrimSpace(authors),
			URL:       e.Request.AbsoluteURL(link),
			Hash:      hash,
		}

		bookListParsed = append(bookListParsed, book)
	}

	// Return the modern parsed results if available, otherwise fall back to legacy parsing
	if len(books) > 0 {
		l.Info("Returning modern parsed results", zap.Int("count", len(books)))
		return books, nil
	}
	
	l.Info("Returning legacy parsed results", zap.Int("count", len(bookListParsed)))
	return bookListParsed, nil
}

func (b *Book) Download(secretKey, folderPath string) error {
	apiURL := fmt.Sprintf(AnnasDownloadEndpoint, b.Hash, secretKey)

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var apiResp fastDownloadResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return err
	}
	if apiResp.DownloadURL == "" {
		if apiResp.Error != "" {
			return errors.New(apiResp.Error)
		}
		return errors.New("failed to get download URL")
	}

	downloadResp, err := httpClient.Get(apiResp.DownloadURL)
	if err != nil {
		return err
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		return errors.New("failed to download file")
	}

	filename := b.Title + "." + b.Format
	filename = strings.ReplaceAll(filename, "/", "_")
	filePath := filepath.Join(folderPath, filename)

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, downloadResp.Body)
	return err
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
