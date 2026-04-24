package anna

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// ErrStalled is the cancellation cause set when a download makes no
// progress for longer than the stall deadline. After io.Copy returns
// an error, callers check errors.Is(context.Cause(ctx), ErrStalled)
// on the ctx returned by stallingGet to distinguish a stall from
// other cancellations.
var ErrStalled = errors.New("download stalled: no bytes received within stall timeout")

// stallingGet issues a GET with TTFB-only header deadline and a
// stall-based body deadline that resets on every successful read.
// Unlike http.Client.Timeout, which bounds the entire exchange
// including body reads, there is no upper bound on transfer
// duration — only on inactivity.
//
// The returned ctx is derived from parent and carries the stall
// watchdog's cancellation. The returned body wraps resp.Body so that
// every successful Read resets the stall timer. The caller must both
// Close resp.Body and invoke cleanup() to release the watchdog.
func stallingGet(
	parent context.Context,
	url, userAgent string,
	headerTimeout, stallTimeout time.Duration,
) (context.Context, *http.Response, io.Reader, func(), error) {
	ctx, cancelCause := context.WithCancelCause(parent)

	client := &http.Client{
		Transport: &http.Transport{ResponseHeaderTimeout: headerTimeout},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancelCause(nil)
		return ctx, nil, nil, nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		cancelCause(nil)
		return ctx, nil, nil, nil, err
	}

	timer := time.AfterFunc(stallTimeout, func() { cancelCause(ErrStalled) })
	body := &stallReader{r: resp.Body, bump: func() { timer.Reset(stallTimeout) }}
	cleanup := func() {
		timer.Stop()
		cancelCause(nil)
	}
	return ctx, resp, body, cleanup, nil
}

// stallReader invokes bump after every successful Read.
type stallReader struct {
	r    io.Reader
	bump func()
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.bump()
	}
	return n, err
}
