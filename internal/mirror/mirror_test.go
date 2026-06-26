package mirror

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestParseStatusPageExtractsAnnaCandidatesWithoutHardcodedIDs(t *testing.T) {
	t.Parallel()

	html := `
<html><body>
<script>
window.preloadData = {'config':{'slug':'slum'},'incident':null,'publicGroupList':[
  {'id':1,'name':'Anna\'s Archive','monitorList':[
    {'id':900,'name':'Anna A','sendUrl':1,'type':'keyword','url':'https://annas-archive.zz/'},
    {'id':901,'name':'Anna B','sendUrl':1,'type':'keyword','url':'https://annas-archive.yy/'}
  ]},
  {'id':2,'name':'Others','monitorList':[
    {'id':999,'name':'Not Anna','sendUrl':1,'type':'keyword','url':'https://example.org/'}
  ]}
],'maintenanceList':[]};
</script>
</body></html>`

	candidates, err := ParseStatusPageHTML(html)
	if err != nil {
		t.Fatalf("ParseStatusPageHTML returned error: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 Anna candidates, got %d", len(candidates))
	}

	if candidates[0].MonitorID != 900 || candidates[0].BaseURL != "annas-archive.zz" {
		t.Fatalf("unexpected first candidate: %+v", candidates[0])
	}

	if candidates[1].MonitorID != 901 || candidates[1].BaseURL != "annas-archive.yy" {
		t.Fatalf("unexpected second candidate: %+v", candidates[1])
	}
}

func TestResolveBestBaseURLPrefersHealthyFastCandidateAndSkipsFailedProbe(t *testing.T) {
	t.Parallel()

	html := `
<script>
window.preloadData = {'config':{'slug':'slum'},'incident':null,'publicGroupList':[
  {'id':1,'name':'Anna\'s Archive','monitorList':[
    {'id':10,'name':'Slow','sendUrl':1,'type':'keyword','url':'https://annas-archive.slow/'},
    {'id':20,'name':'FastButProbeFails','sendUrl':1,'type':'keyword','url':'https://annas-archive.fail/'},
    {'id':30,'name':'FastHealthy','sendUrl':1,'type':'keyword','url':'https://annas-archive.good/'}
  ]}
],'maintenanceList':[]};
</script>`

	heartbeatJSON := `{
		"heartbeatList": {
			"10": [
				{"status": 1, "time": "2026-03-23 10:00:00", "ping": 900},
				{"status": 1, "time": "2026-03-23 10:01:00", "ping": 800}
			],
			"20": [
				{"status": 1, "time": "2026-03-23 10:00:00", "ping": 100},
				{"status": 1, "time": "2026-03-23 10:01:00", "ping": 110}
			],
			"30": [
				{"status": 1, "time": "2026-03-23 10:00:00", "ping": 150},
				{"status": 1, "time": "2026-03-23 10:01:00", "ping": 140}
			]
		},
		"uptimeList": {}
	}`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://status.example/":
				return stringResponse(req, http.StatusOK, html), nil
			case "https://status.example/api/status-page/heartbeat/slum":
				return stringResponse(req, http.StatusOK, heartbeatJSON), nil
			default:
				return nil, errors.New("unexpected request: " + req.URL.String())
			}
		}),
	}

	probeCalls := make([]string, 0)
	resolver := NewResolver(client, "https://status.example/", func(ctx context.Context, baseURL string) error {
		probeCalls = append(probeCalls, baseURL)
		if baseURL == "annas-archive.fail" {
			return errors.New("probe failed")
		}
		return nil
	})

	baseURL, err := resolver.Resolve(context.Background(), ResolveOptions{FallbackBaseURL: "fallback.example"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if baseURL != "annas-archive.good" {
		t.Fatalf("expected annas-archive.good, got %q", baseURL)
	}

	if len(probeCalls) == 0 {
		t.Fatal("expected probe to be called")
	}
}

func TestResolveProbesURLFallbackCandidatesWithoutHeartbeatData(t *testing.T) {
	t.Parallel()

	html := `<html><body><a href="https://annas-archive.fallback/">mirror</a></body></html>`
	heartbeatJSON := `{"heartbeatList":{},"uptimeList":{}}`

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://status.example/":
				return stringResponse(req, http.StatusOK, html), nil
			case "https://status.example/api/status-page/heartbeat/slum":
				return stringResponse(req, http.StatusOK, heartbeatJSON), nil
			default:
				return nil, errors.New("unexpected request: " + req.URL.String())
			}
		}),
	}

	resolver := NewResolver(client, "https://status.example/", func(ctx context.Context, baseURL string) error {
		if baseURL != "annas-archive.fallback" {
			return errors.New("unexpected probe: " + baseURL)
		}
		return nil
	})

	baseURL, err := resolver.Resolve(context.Background(), ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if baseURL != "annas-archive.fallback" {
		t.Fatalf("expected annas-archive.fallback, got %q", baseURL)
	}
}

func TestResolveFallsBackWhenStatusPageFails(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		}),
	}

	resolver := NewResolver(client, "https://status.example/", func(ctx context.Context, baseURL string) error {
		return nil
	})

	baseURL, err := resolver.Resolve(context.Background(), ResolveOptions{FallbackBaseURL: "fallback.example"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if baseURL != "fallback.example" {
		t.Fatalf("expected fallback.example, got %q", baseURL)
	}
}

func stringResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}
}

func TestCandidateScoreUsesRecentHeartbeatQuality(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 11, 0, 0, 0, time.UTC)
	candidate := Candidate{
		BaseURL: "annas-archive.good",
		Heartbeats: []Heartbeat{
			{Status: 1, Ping: 200, Time: now.Add(-2 * time.Minute)},
			{Status: 1, Ping: 100, Time: now.Add(-1 * time.Minute)},
			{Status: 0, Ping: 0, Time: now},
		},
	}

	score := candidate.Score()
	if score.SuccessCount != 2 {
		t.Fatalf("expected 2 successful heartbeats, got %d", score.SuccessCount)
	}

	if score.LastStatus != 0 {
		t.Fatalf("expected last status 0, got %d", score.LastStatus)
	}

	if score.AveragePing != 150 {
		t.Fatalf("expected average ping 150, got %d", score.AveragePing)
	}
}
