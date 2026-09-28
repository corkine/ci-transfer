package transfer

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
)

func TestRetryBoundsAndPermanentErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		err           error
		retries, want int
	}{
		{"transient", io.EOF, 3, 4},
		{"disabled", io.EOF, 0, 1},
		{"forbidden", &oss.ServiceError{StatusCode: 403}, 3, 1},
		{"unauthorized", &oss.ServiceError{StatusCode: 401}, 3, 1},
		{"bad request", &oss.ServiceError{StatusCode: 400}, 3, 1},
		{"throttle", &oss.ServiceError{StatusCode: 429}, 3, 4},
		{"local missing", &os.PathError{Op: "open", Path: "missing", Err: os.ErrNotExist}, 3, 1},
		{"deadline", context.DeadlineExceeded, 3, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := 0
			err := withRetry(context.Background(), Options{Retries: tc.retries, RetryDelay: time.Nanosecond}, tc.name, func(context.Context) error { n++; return tc.err })
			if !errors.Is(err, tc.err) || n != tc.want {
				t.Fatalf("attempts=%d want=%d error=%v", n, tc.want, err)
			}
		})
	}
}

func TestRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	err := withRetry(ctx, Options{Retries: 3, RetryDelay: time.Hour}, "cancel", func(context.Context) error { n++; cancel(); return io.EOF })
	if !errors.Is(err, context.Canceled) || n != 1 {
		t.Fatalf("attempts=%d error=%v", n, err)
	}
}
