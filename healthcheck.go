package healthcheck

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bigmate/app"
	"github.com/hashicorp/go-multierror"
)

type Resource interface {
	Name() string
	Ping(ctx context.Context) error
}

type healthCheck struct {
	port       string
	path       string
	timeout    time.Duration
	resources  []Resource
	httpServer *http.Server
}

func New(ctx context.Context, options ...Option) app.App {
	hc := defaultHealthCheck()

	for _, apply := range options {
		apply(hc)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(hc.path, hc.pingHandler())

	s := &http.Server{
		Addr:    ":" + hc.port,
		Handler: mux,
		BaseContext: func(listener net.Listener) context.Context {
			return ctx
		},
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return ctx
		},
	}

	hc.httpServer = s

	return hc
}

func (hc *healthCheck) Run(ctx context.Context) error {
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	multiErr := &multierror.Error{}

	wg.Add(1)

	go func() {
		defer wg.Done()
		<-ctx.Done()
		mu.Lock()
		defer mu.Unlock()
		if err := hc.httpServer.Shutdown(context.Background()); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}()

	if err := hc.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		mu.Lock()
		defer mu.Unlock()
		multiErr = multierror.Append(multiErr, err)
	}

	wg.Wait()

	return multiErr.ErrorOrNil()
}

type (
	resStat struct {
		Name  string `json:"name,omitempty"`
		Error string `json:"error,omitempty"`
	}
	pingResult struct {
		Ok        bool      `json:"ok"`
		Resources []resStat `json:"resources"`
	}
)

func (hc *healthCheck) pingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), hc.timeout)
		defer cancel()

		mu := sync.Mutex{}
		wg := sync.WaitGroup{}
		pr := pingResult{Ok: true}

		wg.Add(len(hc.resources))

		for _, res := range hc.resources {
			go func(res Resource) {
				defer wg.Done()
				if err := res.Ping(ctx); err != nil {
					mu.Lock()
					pr.Ok = false
					pr.Resources = append(pr.Resources, resStat{
						Name:  res.Name(),
						Error: err.Error(),
					})
					mu.Unlock()
				}
			}(res)
		}

		wg.Wait()

		reportStatus(w, pr)
	}
}

func reportStatus(w http.ResponseWriter, result pingResult) {
	bs, err := json.Marshal(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-type", "application/json")
	w.Write(bs)
}
