package transfer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOSSHTTPRetries(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		status, retries, want int
		success               bool
	}{
		{"retry 503 and replay body", 503, 3, 3, true},
		{"retry 429", 429, 3, 3, true},
		{"403 is final", 403, 3, 1, false},
		{"disable retries", 503, 0, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count atomic.Int32
			payload := strings.Repeat("zip data", 4096)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := count.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != payload {
					t.Errorf("upload body changed on attempt %d: %v", n, err)
				}
				if r.URL.Path != "/bucket/service/package.zip" || r.Method != "PUT" || r.Header.Get("Authorization") == "" {
					t.Errorf("incorrect request: %s %s", r.Method, r.URL.Path)
				}
				if n < 3 {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(tc.status)
					io.WriteString(w, "<Error><Code>TestFailure</Code><Message>injected failure</Message><RequestId>test</RequestId></Error>")
					return
				}
				w.Header().Set("ETag", "test")
				w.WriteHeader(200)
			}))
			defer server.Close()
			file := writeTestFile(t, filepath.Join(t.TempDir(), "package.zip"), payload)
			config, _ := json.Marshal(ossConfig{Bucket: "bucket", Endpoint: server.URL, KeyID: "id", KeySecret: "secret", Path: "/service/"})
			a := Options{OSS: string(config), Sources: []string{file}, Retries: tc.retries, RetryDelay: time.Millisecond, Timeout: time.Second, ConnectTimeout: time.Second}
			err := handleOSS(context.Background(), a)
			if (err == nil) != tc.success || int(count.Load()) != tc.want {
				t.Fatalf("requests=%d want=%d error=%v", count.Load(), tc.want, err)
			}
		})
	}
}

func TestOSSAttemptTimeoutRetries(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if count.Add(1) == 1 {
			<-r.Context().Done()
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	file := writeTestFile(t, filepath.Join(t.TempDir(), "file"), "data")
	config, _ := json.Marshal(ossConfig{Bucket: "bucket", Endpoint: server.URL, KeyID: "id", KeySecret: "secret", Path: "file"})
	err := handleOSS(context.Background(), Options{OSS: string(config), Sources: []string{file}, Retries: 1, RetryDelay: time.Millisecond, Timeout: 100 * time.Millisecond, ConnectTimeout: time.Second})
	if err != nil || count.Load() != 2 {
		t.Fatalf("count=%d err=%v", count.Load(), err)
	}
}
