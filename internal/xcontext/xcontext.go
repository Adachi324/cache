// This file is copied from https://github.com/golang/tools/blob/master/internal/xcontext/xcontext.go
// Since xcontext is an internal package, we are not allowed to directly use it.

package xcontext

import (
	"context"
	"time"
)

// Detach returns a context that keeps all the values of its parent context
// but detaches from the cancellation and error handling.
func Detach(ctx context.Context) context.Context {
	return detachedContext{ctx} //return new context with parent's context value
}

type detachedContext struct {
	parent context.Context
}

func (v detachedContext) Deadline() (time.Time, bool)       { return time.Time{}, false }
func (v detachedContext) Done() <-chan struct{}             { return nil }
func (v detachedContext) Err() error                        { return nil }
func (v detachedContext) Value(key interface{}) interface{} { return v.parent.Value(key) }
