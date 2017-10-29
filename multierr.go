package agentman

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type MultiErr struct {
	m    sync.Mutex
	errs []error
}

func NewMultiErr() *MultiErr {
	me := &MultiErr{
		errs: make([]error, 0),
	}
	return me
}

// Size returns the entire size of this multi error
func (e *MultiErr) Size() int {
	e.m.Lock()
	defer e.m.Unlock()
	return len(e.errs)
}

// Add will add an error, ignoring nils
func (e *MultiErr) Add(err error) {
	e.m.Lock()
	defer e.m.Unlock()
	if err != nil {
		e.errs = append(e.errs, err)
	}
}

func (e *MultiErr) Err() error {
	if e.Size() == 0 {
		return nil
	} else {
		return errors.New(e.Error())
	}
}

func (e *MultiErr) Error() string {
	e.m.Lock()
	defer e.m.Unlock()
	errStr := ""
	for _, err := range e.errs {
		errStr = fmt.Sprintf("%s\n%s;", errStr, err)
	}
	return strings.TrimSpace(errStr)
}

func (e *MultiErr) String() string {
	return e.Error()
}
