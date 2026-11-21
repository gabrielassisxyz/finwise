package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gabrielassisxyz/finwise/internal/ledger"
)

// ReadHandler serves one `read` method. args are the raw elements of the
// request's argument array, already counted against the manifest but not
// decoded; the result is encoded as JSON, and nil becomes null. The reader
// opens the snapshot on each call, so a handler holds nothing between calls.
// An error is mapped by its class and its text is never logged or sent.
type ReadHandler func(ctx context.Context, args []json.RawMessage, reader *ledger.Reader) (any, error)

// Registry maps "domain.name" to the handler of a `read` method.
type Registry struct {
	handlers map[string]ReadHandler
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{handlers: map[string]ReadHandler{}}
}

// Register panics on a second handler for one method: that is a wiring mistake
// found at start, not a condition to handle.
func (registry *Registry) Register(method string, handler ReadHandler) {
	if _, taken := registry.handlers[method]; taken {
		panic(fmt.Sprintf("api: read method %s registered twice", method))
	}
	registry.handlers[method] = handler
}

// Names lists the registered methods in order.
func (registry *Registry) Names() []string {
	names := make([]string, 0, len(registry.handlers))
	for name := range registry.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (registry *Registry) lookup(method string) (ReadHandler, bool) {
	handler, found := registry.handlers[method]
	return handler, found
}

// domainRegistrations has one line per domain. A domain's handlers live in a
// file of their own with a function `func registerX(*Registry)` that calls
// Register once per method, so two domains never edit the same lines.
var domainRegistrations = []func(*Registry){
	registerShell,
	registerAccounts,
	registerTransactions,
}

// NewReadRegistry returns the registry every domain's handlers are in.
func NewReadRegistry() *Registry {
	registry := NewRegistry()
	for _, register := range domainRegistrations {
		register(registry)
	}
	return registry
}
