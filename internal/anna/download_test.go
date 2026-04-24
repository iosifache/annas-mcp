package anna

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowSteadyHandler writes chunkSize bytes every tick, for chunkCount
// chunks, flushing after each write. Simulates a legitimate slow
// server: total transfer exceeds any reasonable TTFB timeout, but the
// inter-chunk gap is well under any sane stall timeout.
func slowSteadyHandler(chunkSize, chunkCount int, tick time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "flusher unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", chunkSize*chunkCount))
		w.WriteHeader(http.StatusOK)
		chunk := bytes.Repeat([]byte{'x'}, chunkSize)
		for i := 0; i < chunkCount; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			flusher.Flush()
			select {
			case <-time.After(tick):
			case <-r.Context().Done():
				return
			}
		}
	}
}

// stallAfterHeadersHandler sends 200 OK + headers, flushes, then
// blocks until the client disconnects.
func stallAfterHeadersHandler(w http.ResponseWriter, r *http.Request) {
	flusher, _ := w.(http.Flusher)
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	<-r.Context().Done()
}

// silentHandler accepts the connection without ever writing a response.
func silentHandler(_ http.ResponseWriter, r *http.Request) {
	<-r.Context().Done()
}

func TestStallingGet_SlowSteadyStreamSucceeds(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(slowSteadyHandler(1024, 30, 100*time.Millisecond))
	defer srv.Close()

	ctx, resp, body, cleanup, err := stallingGet(
		context.Background(), srv.URL, "", 1*time.Second, 500*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	defer resp.Body.Close()

	var buf bytes.Buffer
	n, err := io.Copy(&buf, body)
	if err != nil {
		t.Fatalf("unexpected copy error: %v (cause: %v)", err, context.Cause(ctx))
	}
	if n != 30*1024 {
		t.Fatalf("short write: got %d, want %d", n, 30*1024)
	}
}

func TestStallingGet_StallFires(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(stallAfterHeadersHandler))
	defer srv.Close()

	start := time.Now()
	ctx, resp, body, cleanup, err := stallingGet(
		context.Background(), srv.URL, "", 2*time.Second, 200*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	defer resp.Body.Close()

	_, copyErr := io.Copy(io.Discard, body)
	elapsed := time.Since(start)

	if copyErr == nil {
		t.Fatalf("expected copy error on stall, got nil")
	}
	if cause := context.Cause(ctx); !errors.Is(cause, ErrStalled) {
		t.Fatalf("expected context.Cause(ctx) == ErrStalled, got: %v", cause)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("stall took too long to fire: %v", elapsed)
	}
}

func TestStallingGet_HeaderTimeoutFires(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(silentHandler))
	defer srv.Close()

	start := time.Now()
	_, _, _, _, err := stallingGet(
		context.Background(), srv.URL, "", 200*time.Millisecond, 10*time.Second,
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected header timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("header timeout took too long to fire: %v", elapsed)
	}
}

func TestStallingGet_ExposesHeadersBeforeRead(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="example.pdf"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-1.4 fake"))
	}))
	defer srv.Close()

	_, resp, body, cleanup, err := stallingGet(
		context.Background(), srv.URL, "", 2*time.Second, 2*time.Second,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Disposition"); got == "" {
		t.Fatalf("expected Content-Disposition header, got empty")
	}
	b, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("unexpected body: %q", b)
	}
}

func TestStallingGet_ExternalCancellation(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(slowSteadyHandler(1024, 30, 100*time.Millisecond))
	defer srv.Close()

	parent, cancelParent := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancelParent)

	ctx, resp, body, cleanup, err := stallingGet(
		parent, srv.URL, "", 2*time.Second, 2*time.Second,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	defer resp.Body.Close()

	_, copyErr := io.Copy(io.Discard, body)
	if copyErr == nil {
		t.Fatalf("expected cancellation error")
	}
	if cause := context.Cause(ctx); errors.Is(cause, ErrStalled) {
		t.Fatalf("external cancellation should not surface as ErrStalled, got cause: %v", cause)
	}
}

// Regression test for the bug the PR fixes: a legitimate slow stream
// that would be killed by a whole-request http.Client.Timeout must
// succeed when driven by stallingGet, since the watchdog resets on
// every successful read.
func TestStallingGet_ProgressResetsWatchdog(t *testing.T) {
	t.Parallel()
	// 3 seconds of total transfer, 100ms gaps, 250ms stall timeout.
	// A whole-request timeout of 250ms would abort immediately.
	srv := httptest.NewServer(slowSteadyHandler(512, 30, 100*time.Millisecond))
	defer srv.Close()

	_, resp, body, cleanup, err := stallingGet(
		context.Background(), srv.URL, "", 1*time.Second, 250*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	defer resp.Body.Close()

	var buf bytes.Buffer
	n, err := io.Copy(&buf, body)
	if err != nil {
		t.Fatalf("stall watchdog fired on steady stream: %v", err)
	}
	if n != 30*512 {
		t.Fatalf("short write: got %d, want %d", n, 30*512)
	}
}
