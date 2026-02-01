package anna

import (
	"fmt"
	"net/url"

	"strings"

	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	colly "github.com/gocolly/colly/v2"
	"github.com/iosifache/annas-mcp/internal/env"
	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

const (
	// content=book_any forces server-side rendering of results (required for scraping)
	AnnasSearchEndpointFormat   = "https://%s/search?q=%s&content=book_any"
	AnnasDownloadEndpointFormat = "https://%s/dyn/api/fast_download.json?md5=%s&key=%s"
)

func extractMetaInformation(meta string) (language, format, size string) {
	// The meta format may be:
	// - "✅ English [en] · EPUB · 0.7MB · 2015 · ..."
	// - "✅ English [en] · Hindi [hi] · EPUB · 0.7MB · ..."
	parts := strings.Split(meta, " · ")
	if len(parts) < 3 {
		return "", "", ""
	}

	languagePart := strings.TrimSpace(parts[0])
	if idx := strings.Index(languagePart, "["); idx > 0 {
		language = strings.TrimSpace(languagePart[:idx])
		language = strings.TrimLeft(language, "✅ ")
	}

	// Format is typically all caps (EPUB, PDF, MOBI, etc.). Size contains MB, KB, GB.
	formatIdx := -1
	sizeIdx := -1

	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])
		if strings.Contains(part, "MB") || strings.Contains(part, "KB") || strings.Contains(part, "GB") {
			sizeIdx = i
			if formatIdx == -1 && i > 0 {
				formatIdx = i - 1
			}
			break
		}
	}

	if formatIdx > 0 && formatIdx < len(parts) {
		format = strings.TrimSpace(parts[formatIdx])
	}

	if sizeIdx > 0 && sizeIdx < len(parts) {
		size = strings.TrimSpace(parts[sizeIdx])
	}

	return language, format, size
}

func FindBook(query string) ([]*Book, error) {
	l := logger.GetLogger()

	c := colly.NewCollector(
		colly.Async(true),
		colly.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	bookList := make([]*colly.HTMLElement, 0)

	c.OnHTML("a[href^='/md5/']", func(e *colly.HTMLElement) {
		// Only process the first link (the cover image link), not the duplicate title link
		if e.Attr("class") == "custom-a block mr-2 sm:mr-4 hover:opacity-80" {
			bookList = append(bookList, e)
		}
	})

	c.OnRequest(func(r *colly.Request) {
		l.Info("Visiting URL", zap.String("url", r.URL.String()))
	})

	env, err := env.GetEnv()
	if err != nil {
		return nil, err
	}

	annasSearchEndpoint := fmt.Sprintf(AnnasSearchEndpointFormat, env.AnnasBaseURL)
	fullURL := fmt.Sprintf(annasSearchEndpoint, url.QueryEscape(query))
	c.Visit(fullURL)
	c.Wait()

	bookListParsed := make([]*Book, 0)
	for _, e := range bookList {
		bookInfoDiv := e.DOM.Parent().Find("div.max-w-full")

		title := bookInfoDiv.Find("a[href^='/md5/']").Text()

		authorsRaw := bookInfoDiv.Find("a[href^='/search'] span.icon-\\[mdi--user-edit\\]").Parent().Text()
		authors := strings.TrimSpace(authorsRaw)

		publisherRaw := bookInfoDiv.Find("a[href^='/search'] span.icon-\\[mdi--company\\]").Parent().Text()
		publisher := strings.TrimSpace(publisherRaw)

		meta := bookInfoDiv.Find("div.text-gray-800").Text()

		language, format, size := extractMetaInformation(meta)

		link := e.Attr("href")
		hash := strings.TrimPrefix(link, "/md5/")

		book := &Book{
			Language:  language,
			Format:    format,
			Size:      size,
			Title:     strings.TrimSpace(title),
			Publisher: publisher,
			Authors:   authors,
			URL:       e.Request.AbsoluteURL(link),
			Hash:      hash,
		}

		bookListParsed = append(bookListParsed, book)
	}

	return bookListParsed, nil
}

func (b *Book) Download(secretKey, folderPath string) error {
	env, err := env.GetEnv()
	if err != nil {
		return err
	}

	annasDownloadEndpoint := fmt.Sprintf(AnnasDownloadEndpointFormat, env.AnnasBaseURL)
	apiURL := fmt.Sprintf(annasDownloadEndpoint, b.Hash, secretKey)

	resp, err := http.Get(apiURL)
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

	downloadResp, err := http.Get(apiResp.DownloadURL)
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
