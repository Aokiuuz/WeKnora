package docparser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestMinerUResultDownloadRecoversInterruptedBody(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		if calls.Add(1) == 1 {
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "interrupted")
			return
		}
		fmt.Fprint(w, "complete result archive")
	}))
	defer server.Close()
	data, err := downloadMinerUArchive(context.Background(), server.Client(), server.URL)
	if err != nil || string(data) != "complete result archive" || calls.Load() != 2 {
		t.Fatalf("data=%q err=%v calls=%d", data, err, calls.Load())
	}
}

func TestMinerUResultDownloadCancellationStopsRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := downloadMinerUArchive(ctx, server.Client(), server.URL)
	if !errors.Is(err, context.DeadlineExceeded) || calls.Load() != 1 || time.Since(start) > time.Second {
		t.Fatalf("err=%v calls=%d elapsed=%v", err, calls.Load(), time.Since(start))
	}
}

func TestMinerUResultDownloadRetryBounds(t *testing.T) {
	for _, tt := range []struct{ status, calls int }{{403, 1}, {503, 2}} {
		t.Run(fmt.Sprint(tt.status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			_, err := downloadMinerUArchive(context.Background(), server.Client(), server.URL)
			if err == nil || calls.Load() != int32(tt.calls) {
				t.Fatalf("err=%v calls=%d", err, calls.Load())
			}
		})
	}
}
