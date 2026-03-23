package github

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
}

func newRetryTransport(base http.RoundTripper) http.RoundTripper {
	return &retryTransport{base: base, maxRetries: 3}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := range t.maxRetries {
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		if !shouldRetry(resp.StatusCode) {
			return resp, nil
		}

		wait := retryDelay(resp, attempt)
		resp.Body.Close()
		time.Sleep(wait)
	}

	return resp, err
}

func shouldRetry(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable
}

func retryDelay(resp *http.Response, attempt int) time.Duration {
	if resp.StatusCode == http.StatusTooManyRequests {
		if after := resp.Header.Get("Retry-After"); after != "" {
			if secs, err := strconv.Atoi(after); err == nil {
				return time.Duration(secs) * time.Second
			}
		}
		if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
			if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
				wait := time.Until(time.Unix(ts, 0))
				if wait > 0 && wait < 120*time.Second {
					return wait
				}
			}
		}
	}
	return time.Duration(math.Pow(2, float64(attempt))) * time.Second
}
