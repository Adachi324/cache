package group

import "sync"

// Group represents a class of work and forms a namespace in which
// units of work can be executed with duplicate suppression.
type Group struct {
	mu sync.Mutex
	m  map[string]*Call
}

// Call represents an in-flight or completed work
type Call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// NewGroup creates a empty Group
func NewGroup() *Group {
	m := make(map[string]*Call)

	return &Group{
		m: m,
	}
}

// AddCalls tries to add calls and return waitingCallsMap which indicates the key is being handled by others
// and toHandleKeys which indicate the keys to be handled by the caller
func (g *Group) AddCalls(keys []string) (waitingCallsMap map[string]*Call, toHandleKeys []string) {
	waitingCallsMap = make(map[string]*Call)
	toHandleKeys = make([]string, 0, len(keys))

	g.mu.Lock()
	for _, key := range keys {
		if curCall, ok := g.m[key]; ok {
			waitingCallsMap[key] = curCall
		} else {
			toHandleKeys = append(toHandleKeys, key)
			newCall := &Call{}
			newCall.wg.Add(1)
			g.m[key] = newCall
		}
	}
	g.mu.Unlock()

	return waitingCallsMap, toHandleKeys
}

// CompleteCalls completes multiple calls for key value pairs in valueMap
func (g *Group) CompleteCalls(valueMap map[string]interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for key, val := range valueMap {
		g.m[key].val = val
		g.m[key].wg.Done()
		delete(g.m, key)
	}
}

// WaitCall waits a call to complete
func (g *Group) WaitCall(call *Call) (interface{}, error) {
	call.wg.Wait()
	val := call.val
	err := call.err

	return val, err
}
