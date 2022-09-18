package healthcheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/bigmate/healthcheck"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type resource struct {
	sleep   time.Duration
	counter uint32
}

func (r *resource) Name() string {
	return "resource"
}

func (r *resource) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		if r.sleep > 0 {
			time.Sleep(r.sleep)
		}
		atomic.AddUint32(&r.counter, 1)
		return errors.New("failed to ping")
	}
}

func TestHealthcheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	sleep := time.Second * 10
	r := &resource{sleep: sleep}
	h := healthcheck.New(ctx,
		healthcheck.WithTimeout(time.Minute),
		healthcheck.WithSemaphore(5),
		healthcheck.WithResource(r),
		healthcheck.WithResource(r),
		healthcheck.WithResource(r),
		healthcheck.WithResource(r),
		healthcheck.WithResource(r),
	)

	srv := httptest.NewServer(h)
	defer srv.Close()

	start := time.Now()

	res, err := http.Get(srv.URL)
	if err != nil {
		cancel()
		t.Fatalf("failed to issue request: %v", err)
	}

	end := time.Now()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status ok")
	}

	buf := bytes.Buffer{}
	if _, err := io.Copy(&buf, res.Body); err != nil {
		t.Fatalf("failed to copy: %v", err)
	}

	const expected = `
		{
		  "ok": false,
		  "erroredResources": [
			{
			  "name": "resource",
			  "error": "failed to ping"
			},
			{
			  "name": "resource",
			  "error": "failed to ping"
			},
			{
			  "name": "resource",
			  "error": "failed to ping"
			},
			{
			  "name": "resource",
			  "error": "failed to ping"
			},
			{
			  "name": "resource",
			  "error": "failed to ping"
			}
		  ]
		}`

	des := bytes.Buffer{}

	if err := json.Compact(&des, []byte(expected)); err != nil {
		t.Fatalf("failed to compact expected: %v", err)
	}

	if bytes.Compare(buf.Bytes(), des.Bytes()) != 0 {
		t.Fatalf("body is not equal expected: %s", buf.String())
	}

	counter := atomic.LoadUint32(&r.counter)
	if counter != 5 {
		t.Fatalf("counter is not equal to 5, got: %v", counter)
	}

	if math.Abs(float64(end.Sub(start)-sleep)) > float64(time.Millisecond*500) {
		t.Fatalf("pinging took %v", end.Sub(start))
	}
}
