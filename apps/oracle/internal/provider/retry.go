package provider

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Retry policy for provider HTTP calls. The values are variables so tests can
// shrink the delays.
var (
	retryMaxAttempts = 3
	retryBaseDelay   = 500 * time.Millisecond
	retryMaxDelay    = 10 * time.Second
)

// isRetryableStatus reports whether a response status is worth retrying:
// 429 Too Many Requests and any 5xx.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

// doWithRetry sends the request built by newReq, retrying up to
// retryMaxAttempts times on transport errors, 429 and 5xx responses with
// jittered exponential backoff (honouring a Retry-After seconds header, capped
// at retryMaxDelay). It stops early when ctx is done. The final response (which
// may still carry a retryable status) or error is returned; the caller owns
// closing the response body.
func doWithRetry(ctx context.Context, client *http.Client, newReq func() (*http.Request, error)) (*http.Response, error) {
	for attempt := 1; ; attempt++ {
		req, err := newReq()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		last := attempt >= retryMaxAttempts || ctx.Err() != nil
		if err != nil {
			if last {
				return nil, err
			}
		} else if !isRetryableStatus(resp.StatusCode) || last {
			return resp, nil
		}

		delay := backoffDelay(attempt)
		if resp != nil {
			if ra := retryAfter(resp); ra > 0 {
				delay = min(ra, retryMaxDelay)
			}
			// Drain and close so the connection can be reused.
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			_ = resp.Body.Close()
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// backoffDelay returns a jittered delay for the given 1-based attempt: a
// random duration in [d/2, d] where d = retryBaseDelay * 2^(attempt-1), capped
// at retryMaxDelay.
func backoffDelay(attempt int) time.Duration {
	d := retryBaseDelay << (attempt - 1)
	if d > retryMaxDelay || d <= 0 {
		d = retryMaxDelay
	}
	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// retryAfter parses a Retry-After header given in seconds (0 when absent or
// not a positive integer).
func retryAfter(resp *http.Response) time.Duration {
	secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
