package errutil

import "sync"

type ErrorDone struct {
	doneOnce sync.Once
	doneCh   chan struct{}
	doneFn   func()

	err struct {
		sync.RWMutex
		error
	}
}

func NewErrorDone(onDone func()) *ErrorDone {
	return &ErrorDone{
		doneCh: make(chan struct{}),
		doneFn: onDone,
	}
}

func (e *ErrorDone) SendError(err error) {
	if err == nil {
		return
	}
	e.doneOnce.Do(func() {
		e.err.Lock()
		e.err.error = err
		e.err.Unlock()
		close(e.doneCh)
		if e.doneFn != nil {
			e.doneFn()
		}
	})
}

func (e *ErrorDone) Done() <-chan struct{} {
	return e.doneCh
}

func (e *ErrorDone) Err() error {
	e.err.RLock()
	defer e.err.RUnlock()
	return e.err.error
}
