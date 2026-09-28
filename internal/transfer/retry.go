package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

// Retry only transport failures and transient HTTP responses, never authentication,
// configuration, local filesystem errors, or remote command exit statuses.
func retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var status interface{ HttpStatusCode() int }
	if errors.As(err, &status) {
		n := status.HttpStatusCode()
		return n == 408 || n == 429 || n >= 500 && n <= 599
	}
	var fileErr *os.PathError
	if errors.As(err, &fileErr) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) && dns.IsNotFound {
		return false
	}
	var network net.Error
	if errors.As(err, &network) && (network.Timeout() || network.Temporary()) {
		return true
	}
	// SSH and Windows sometimes wrap transport errors without an Unwrap method.
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"connection reset", "connection refused", "broken pipe", "forcibly closed", "connection timed out", "use of closed network connection", "unexpected eof", "handshake failed: eof"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func withRetry(ctx context.Context, a Options, label string, operation func(context.Context) error) error {
	delay := min(a.RetryDelay, 30*time.Second)
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := operation(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt >= a.Retries || !retryable(err) {
			return fmt.Errorf("%s (attempt %d/%d): %w", label, attempt+1, a.Retries+1, err)
		}
		fmt.Fprintf(os.Stderr, "%s: attempt %d/%d failed: %v; retrying in %s\n", label, attempt+1, a.Retries+1, err, delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}
