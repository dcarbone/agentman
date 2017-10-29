package agentman

import (
	"fmt"
	"strings"
	"sync"
)

type MultiErr struct {
	m    sync.Mutex
	errs []error
}

func NewMultiErr(err error) *MultiErr {
	me := &MultiErr{
		errs: make([]error, 0),
	}
	me.Add(err)
	return me
}

// ErrCount will return a count of how many of contained errors are not nil
func (e *MultiErr) ErrCount() int {
	e.m.Lock()
	defer e.m.Unlock()
	errLen := 0
	for _, err := range e.errs {
		if err != nil {
			errLen++
		}
	}
	return errLen
}

// Size returns the entire size of this multi error, including nils
func (e *MultiErr) Size() int {
	e.m.Lock()
	defer e.m.Unlock()
	return len(e.errs)
}

// Add will add an error even if nil
func (e *MultiErr) Add(err error) {
	e.m.Lock()
	defer e.m.Unlock()
	e.errs = append(e.errs, err)
}

func (e *MultiErr) Error() string {
	e.m.Lock()
	defer e.m.Unlock()
	errStr := ""
	for i, err := range e.errs {
		if err == nil {
			continue
		}
		errStr = fmt.Sprintf("%s %d - %s;", errStr, i, e.errs[i])
	}
	return strings.TrimSpace(errStr)
}

func (e *MultiErr) String() string {
	return e.Error()
}
