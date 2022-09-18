package healthcheck

import (
	"time"
)

type Option func(hc *Healthcheck)

func nop(hc *Healthcheck) {}

func WithPort(port string) Option {
	return func(hc *Healthcheck) {
		hc.port = port
	}
}

func WithPath(path string) Option {
	return func(hc *Healthcheck) {
		hc.path = path
	}
}

func WithResource(resource Resource) Option {
	return func(hc *Healthcheck) {
		hc.resources = append(hc.resources, resource)
	}
}

func WithTimeout(timeout time.Duration) Option {
	if timeout <= 0 {
		return nop
	}

	return func(hc *Healthcheck) {
		hc.timeout = timeout
	}
}

func WithServerShutdownTimeout(timeout time.Duration) Option {
	if timeout <= 0 {
		return nop
	}

	return func(hc *Healthcheck) {
		hc.shutdownTimeout = timeout
	}
}

func WithSemaphore(n int) Option {
	return func(hc *Healthcheck) {
		hc.semaphore = n
	}
}

func defaultHealthCheck() *Healthcheck {
	return &Healthcheck{
		port:            "8082",
		path:            "/health",
		timeout:         time.Second * 10,
		shutdownTimeout: time.Second * 10,
		semaphore:       -1,
	}
}
