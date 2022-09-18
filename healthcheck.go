package healthcheck

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
)

type Resource interface {
	Name() string
	Ping(ctx context.Context) error
}

type Healthcheck struct {
	port            string
	path            string
	semaphore       int
	timeout         time.Duration
	shutdownTimeout time.Duration
	resources       []Resource
	httpServer      *http.Server
}

func New(ctx context.Context, options ...Option) *Healthcheck {
	hc := defaultHealthCheck()

	for _, apply := range options {
		apply(hc)
	}

	if hc.semaphore > len(hc.resources) {
		hc.semaphore = len(hc.resources)
	}

	if hc.semaphore <= 0 {
		hc.semaphore = -1
	}

	mux := http.NewServeMux()
	mux.HandleFunc(hc.path, hc.HandlerFunc)

	s := &http.Server{
		Addr:    ":" + hc.port,
		Handler: mux,
		BaseContext: func(listener net.Listener) context.Context {
			return ctx
		},
	}

	hc.httpServer = s

	return hc
}

func (hc *Healthcheck) Run(ctx context.Context) error {
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	multiErr := &multierror.Error{}

	wg.Add(1)

	go func() {
		defer wg.Done()
		<-ctx.Done()
		mu.Lock()
		defer mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), hc.shutdownTimeout)
		defer cancel()

		if err := hc.httpServer.Shutdown(ctx); err != nil {
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
		Name  string `json:"name"`
		Error string `json:"error"`
	}
	pingResult struct {
		Ok        bool      `json:"ok"`
		Resources []resStat `json:"erroredResources"`
	}
)

func (hc *Healthcheck) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), hc.timeout)
	defer cancel()

	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	pr := pingResult{Ok: true, Resources: make([]resStat, 0)}
	sm := newSemaphore(hc.semaphore)

	for _, res := range hc.resources {
		wg.Add(1)
		sm.acquire()

		go func(res Resource) {
			defer wg.Done()
			defer sm.release()

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

func (hc *Healthcheck) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	hc.ServeHTTP(w, r)
}
