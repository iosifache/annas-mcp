package anna

import (
	"fmt"
	"net/url"

	"regexp"
	"strings"

	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/PuerkitoBio/goquery"
	colly "github.com/gocolly/colly/v2"
	"github.com/iosifache/annas-mcp/internal/env"
	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

const (
	AnnasSearchEndpointFormat   = "https://%s/search?q=%s"
	AnnasDownloadEndpointFormat = "https://%s/dyn/api/fast_download.json"
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

func extractFormatFromFilename(filename string) string {
	re := regexp.MustCompile(`(?i)\.(epub|pdf|mobi|azw3|azw|djvu|cbz|cbr|rtf|txt|docx?|fb2|lit)\b`)
	match := re.FindStringSubmatch(filename)
	if len(match) < 2 {
		return ""
	}

	return strings.ToUpper(match[1])
}

func FindBook(query string) ([]*Book, error) {
	l := logger.GetLogger()

	c := colly.NewCollector(
		colly.Async(true),
	)

	bookList := make([]*colly.HTMLElement, 0)

	c.OnHTML("div.js-aarecord-list-outer > div", func(e *colly.HTMLElement) {
		// Collect result items to avoid processing duplicate md5 links per result.
		bookList = append(bookList, e)
	})

	c.OnRequest(func(r *colly.Request) {
		l.Info("Visiting URL", zap.String("url", r.URL.String()))
	})

	c.OnError(func(r *colly.Response, err error) {
		status := 0
		if r != nil {
			status = r.StatusCode
		}
		l.Warn("Search request failed",
			zap.Int("status", status),
			zap.Error(err),
		)
	})

	envVars, err := env.GetEnv()
	if err != nil {
		return nil, err
	}

	annasSearchEndpoint := fmt.Sprintf(AnnasSearchEndpointFormat, envVars.AnnasBaseURL)
	fullURL := fmt.Sprintf(annasSearchEndpoint, url.QueryEscape(query))
	if err := c.Visit(fullURL); err != nil {
		l.Warn("Search visit failed", zap.Error(err))
		return nil, err
	}
	c.Wait()

	bookListParsed := make([]*Book, 0)
	for _, e := range bookList {
		titleCandidates := e.DOM.Find("a[href^='/md5/']")
		if titleCandidates.Length() == 0 {
			continue
		}
		titleLink := titleCandidates.FilterFunction(func(_ int, s *goquery.Selection) bool {
			return strings.TrimSpace(s.Text()) != ""
		}).First()
		if titleLink.Length() == 0 {
			titleLink = titleCandidates.First()
		}
		title := strings.TrimSpace(titleLink.Text())

		authorsSelection := e.DOM.Find("a[href^='/search?q='] span.icon-\\[mdi--user-edit\\]").First()
		if authorsSelection.Length() == 0 {
			authorsSelection = e.DOM.Find("span.icon-\\[mdi--user-edit\\]").First()
		}
		authorsRaw := authorsSelection.Parent().Text()
		authors := strings.TrimSpace(authorsRaw)

		publisherSelection := e.DOM.Find("a[href^='/search?q='] span.icon-\\[mdi--company\\]").First()
		if publisherSelection.Length() == 0 {
			publisherSelection = e.DOM.Find("span.icon-\\[mdi--company\\]").First()
		}
		publisherRaw := publisherSelection.Parent().Text()
		publisher := strings.TrimSpace(publisherRaw)

		meta := e.DOM.Find("div.text-gray-800").First().Text()

		language, format, size := extractMetaInformation(meta)
		if format == "" {
			filenameLine := strings.TrimSpace(e.DOM.Find("div.font-mono").First().Text())
			format = extractFormatFromFilename(filenameLine)
		}

		link, ok := titleLink.Attr("href")
		if !ok || link == "" {
			continue
		}
		hash := strings.TrimPrefix(link, "/md5/")
		if strings.Contains(hash, "/") {
			hash = strings.SplitN(hash, "/", 2)[0]
		}

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
	apiURL := fmt.Sprintf(AnnasDownloadEndpointFormat, env.DefaultAnnasBaseURL)
	params := url.Values{}
	params.Set("md5", b.Hash)
	params.Set("key", secretKey)
	params.Set("path_index", "0")
	params.Set("domain_index", "0")
	apiURL = apiURL + "?" + params.Encode()

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
