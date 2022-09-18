package healthcheck

type semaphore struct {
	bottleneck chan struct{}
}

func newSemaphore(size int) *semaphore {
	s := &semaphore{}

	if size > 0 {
		s.bottleneck = make(chan struct{}, size)
	}

	return s
}

func (s *semaphore) acquire() {
	if cap(s.bottleneck) > 0 {
		s.bottleneck <- struct{}{}
	}
}

func (s *semaphore) release() {
	if cap(s.bottleneck) > 0 {
		<-s.bottleneck
	}
}
