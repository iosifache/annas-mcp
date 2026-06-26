package mirror

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/iosifache/annas-mcp/internal/logger"
	"go.uber.org/zap"
)

const (
	DefaultStatusPageURL  = "https://open-slum.org/"
	defaultStatusPageSlug = "slum"
	defaultProbeTimeout   = 10 * time.Second
	browserUserAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	slugRegex            = regexp.MustCompile(`['"]slug['"]\s*:\s*['"]([^'"]+)['"]`)
	candidateObjectRegex = regexp.MustCompile(`(?s)\{[^{}]*['"]id['"]\s*:\s*([0-9]+)[^{}]*['"]url['"]\s*:\s*['"]([^'"]+)['"][^{}]*\}`)
	fallbackURLRegex     = regexp.MustCompile(`https://annas-archive\.[a-z0-9-]+/?`)
)

type Heartbeat struct {
	Status    int       `json:"status"`
	Ping      int64     `json:"ping"`
	Time      time.Time `json:"-"`
	Timestamp string    `json:"time"`
}

type Candidate struct {
	MonitorID  int
	BaseURL    string
	SourceURL  string
	Heartbeats []Heartbeat
}

type CandidateScore struct {
	SuccessCount int
	SampleCount  int
	LastStatus   int
	AveragePing  int64
}

type ResolveOptions struct {
	FallbackBaseURL string
}

type ProbeFunc func(context.Context, string) error

type Resolver struct {
	client        *http.Client
	statusPageURL string
	probe         ProbeFunc
}

type heartbeatEnvelope struct {
	HeartbeatList map[string][]Heartbeat `json:"heartbeatList"`
}

func NewResolver(client *http.Client, statusPageURL string, probe ProbeFunc) *Resolver {
	if client == nil {
		client = &http.Client{Timeout: defaultProbeTimeout}
	}

	if statusPageURL == "" {
		statusPageURL = DefaultStatusPageURL
	}

	resolver := &Resolver{
		client:        client,
		statusPageURL: statusPageURL,
	}

	if probe == nil {
		resolver.probe = resolver.defaultProbe
	} else {
		resolver.probe = probe
	}

	return resolver
}

func NormalizeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	return value
}

func ParseStatusPageHTML(html string) ([]Candidate, error) {
	normalized := strings.ReplaceAll(html, `\'`, `'`)

	candidates, err := parseCandidatesFromAnnaGroup(normalized)
	if err != nil {
		return nil, err
	}

	if len(candidates) > 0 {
		return candidates, nil
	}

	return parseCandidatesByURLFallback(normalized), nil
}

func (c Candidate) Score() CandidateScore {
	score := CandidateScore{
		SampleCount: len(c.Heartbeats),
		LastStatus:  -1,
	}

	var pingSum int64
	for idx, hb := range c.Heartbeats {
		if idx == len(c.Heartbeats)-1 {
			score.LastStatus = hb.Status
		}

		if hb.Status == 1 {
			score.SuccessCount++
			pingSum += hb.Ping
		}
	}

	if score.SuccessCount > 0 {
		score.AveragePing = pingSum / int64(score.SuccessCount)
	}

	return score
}

func (r *Resolver) Resolve(ctx context.Context, opts ResolveOptions) (string, error) {
	l := logger.GetLogger()
	fallbackBaseURL := NormalizeBaseURL(opts.FallbackBaseURL)

	html, err := r.fetch(ctx, r.statusPageURL)
	if err != nil {
		l.Warn("Failed to fetch mirror status page, using fallback mirror if available",
			zap.String("statusPageURL", r.statusPageURL),
			zap.String("fallbackBaseURL", fallbackBaseURL),
			zap.Error(err),
		)
		if fallbackBaseURL != "" {
			return fallbackBaseURL, nil
		}
		return "", err
	}

	candidates, err := ParseStatusPageHTML(html)
	if err != nil || len(candidates) == 0 {
		l.Warn("Failed to discover Anna mirror candidates, using fallback mirror if available",
			zap.Int("candidateCount", len(candidates)),
			zap.String("fallbackBaseURL", fallbackBaseURL),
			zap.Error(err),
		)
		if fallbackBaseURL != "" {
			return fallbackBaseURL, nil
		}
		if err == nil {
			err = errors.New("no Anna mirror candidates discovered")
		}
		return "", err
	}

	slug := extractStatusPageSlug(html)
	heartbeatURL, err := buildHeartbeatURL(r.statusPageURL, slug)
	if err != nil {
		if fallbackBaseURL != "" {
			return fallbackBaseURL, nil
		}
		return "", err
	}

	heartbeatJSON, err := r.fetch(ctx, heartbeatURL)
	if err != nil {
		l.Warn("Failed to fetch mirror heartbeat data, using fallback mirror if available",
			zap.String("heartbeatURL", heartbeatURL),
			zap.String("fallbackBaseURL", fallbackBaseURL),
			zap.Error(err),
		)
		if fallbackBaseURL != "" {
			return fallbackBaseURL, nil
		}
		return "", err
	}

	envelope := heartbeatEnvelope{}
	if err := json.Unmarshal([]byte(heartbeatJSON), &envelope); err != nil {
		l.Warn("Failed to decode mirror heartbeat data, using fallback mirror if available",
			zap.String("fallbackBaseURL", fallbackBaseURL),
			zap.Error(err),
		)
		if fallbackBaseURL != "" {
			return fallbackBaseURL, nil
		}
		return "", err
	}

	ranked := rankCandidates(applyHeartbeats(candidates, envelope.HeartbeatList))
	l.Info("Resolved Anna mirror candidates",
		zap.Int("discoveredCandidates", len(candidates)),
		zap.Int("healthyCandidates", len(ranked)),
	)
	for _, candidate := range ranked {
		if r.probe != nil {
			if err := r.probe(ctx, candidate.BaseURL); err != nil {
				l.Warn("Anna mirror probe failed",
					zap.String("baseURL", candidate.BaseURL),
					zap.Error(err),
				)
				continue
			}
		}

		l.Info("Selected Anna mirror automatically",
			zap.String("baseURL", candidate.BaseURL),
			zap.Int("monitorID", candidate.MonitorID),
		)
		return candidate.BaseURL, nil
	}

	if fallbackBaseURL != "" {
		l.Warn("No reachable Anna mirror found, using fallback mirror",
			zap.String("fallbackBaseURL", fallbackBaseURL),
		)
		return fallbackBaseURL, nil
	}

	return "", errors.New("no reachable Anna mirror found")
}

func (r *Resolver) defaultProbe(ctx context.Context, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s/search?q=test&content=book_any", NormalizeBaseURL(baseURL)), nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("probe failed with status %d", resp.StatusCode)
	}

	return nil
}

func (r *Resolver) fetch(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func parseCandidatesFromAnnaGroup(html string) ([]Candidate, error) {
	groupIndex := strings.Index(html, "Anna's Archive")
	if groupIndex < 0 {
		return nil, nil
	}

	monitorKeyIndex := strings.Index(html[groupIndex:], "'monitorList':[")
	if monitorKeyIndex < 0 {
		monitorKeyIndex = strings.Index(html[groupIndex:], `"monitorList":[`)
	}
	if monitorKeyIndex < 0 {
		return nil, errors.New("Anna group found but monitor list missing")
	}

	monitorKeyIndex += groupIndex
	listStart := strings.Index(html[monitorKeyIndex:], "[")
	if listStart < 0 {
		return nil, errors.New("monitor list start missing")
	}

	listStart += monitorKeyIndex
	listEnd := findMatchingBracket(html, listStart)
	if listEnd < 0 {
		return nil, errors.New("monitor list end missing")
	}

	return parseCandidatesFromSection(html[listStart : listEnd+1]), nil
}

func parseCandidatesByURLFallback(html string) []Candidate {
	matches := fallbackURLRegex.FindAllString(html, -1)
	seen := make(map[string]struct{})
	candidates := make([]Candidate, 0, len(matches))

	for _, match := range matches {
		baseURL := NormalizeBaseURL(match)
		if _, exists := seen[baseURL]; exists {
			continue
		}
		seen[baseURL] = struct{}{}
		candidates = append(candidates, Candidate{
			BaseURL:   baseURL,
			SourceURL: "https://" + baseURL + "/",
		})
	}

	return candidates
}

func parseCandidatesFromSection(section string) []Candidate {
	matches := candidateObjectRegex.FindAllStringSubmatch(section, -1)
	candidates := make([]Candidate, 0, len(matches))
	seen := make(map[string]struct{})

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		baseURL := NormalizeBaseURL(match[2])
		if !strings.HasPrefix(baseURL, "annas-archive.") {
			continue
		}

		if _, exists := seen[baseURL]; exists {
			continue
		}

		seen[baseURL] = struct{}{}
		candidates = append(candidates, Candidate{
			MonitorID: mustAtoi(match[1]),
			BaseURL:   baseURL,
			SourceURL: match[2],
		})
	}

	return candidates
}

func applyHeartbeats(candidates []Candidate, heartbeatList map[string][]Heartbeat) []Candidate {
	enriched := make([]Candidate, 0, len(candidates))

	for _, candidate := range candidates {
		if candidate.MonitorID > 0 {
			candidate.Heartbeats = heartbeatList[fmt.Sprintf("%d", candidate.MonitorID)]
		}
		enriched = append(enriched, candidate)
	}

	return enriched
}

func rankCandidates(candidates []Candidate) []Candidate {
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		score := candidate.Score()
		if score.LastStatus == 1 || score.SampleCount == 0 {
			filtered = append(filtered, candidate)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		left := filtered[i].Score()
		right := filtered[j].Score()

		if left.SampleCount == 0 || right.SampleCount == 0 {
			if left.SampleCount != right.SampleCount {
				return left.SampleCount > right.SampleCount
			}
			return filtered[i].BaseURL < filtered[j].BaseURL
		}

		leftRate := successRate(left)
		rightRate := successRate(right)
		if leftRate != rightRate {
			return leftRate > rightRate
		}

		if left.AveragePing != right.AveragePing {
			return left.AveragePing < right.AveragePing
		}

		return filtered[i].BaseURL < filtered[j].BaseURL
	})

	return filtered
}

func successRate(score CandidateScore) float64 {
	if score.SampleCount == 0 {
		return 0
	}

	return float64(score.SuccessCount) / float64(score.SampleCount)
}

func extractStatusPageSlug(html string) string {
	match := slugRegex.FindStringSubmatch(strings.ReplaceAll(html, `\'`, `'`))
	if len(match) == 2 {
		return match[1]
	}

	return defaultStatusPageSlug
}

func buildHeartbeatURL(statusPageURL, slug string) (string, error) {
	parsed, err := url.Parse(statusPageURL)
	if err != nil {
		return "", err
	}

	heartbeatPath := fmt.Sprintf("/api/status-page/heartbeat/%s", slug)
	parsed.Path = heartbeatPath
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}

func findMatchingBracket(input string, start int) int {
	depth := 0
	for idx := start; idx < len(input); idx++ {
		switch input[idx] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return idx
			}
		}
	}

	return -1
}

func mustAtoi(raw string) int {
	value := 0
	for _, ch := range raw {
		value = value*10 + int(ch-'0')
	}
	return value
}
