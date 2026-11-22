package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// ReadHandler serves one `read` method or one `evaluate` method. args are the
// raw elements of the request's argument array, already counted against the
// manifest but not decoded; the result is encoded as JSON, and nil becomes
// null. Every query of one call goes through views, which are opened for the
// request and closed after it, so a handler holds nothing between calls.
// An error is mapped by its class and its text is never logged or sent.
type ReadHandler func(ctx context.Context, args []json.RawMessage, views Views) (any, error)

// Registry maps "domain.name" to the handler of a method of any class but
// `session`. A handler has one of three shapes, which differ in what the router
// opens for it and so in what it may do: ReadHandler, WriteHandler and
// JournalHandler.
type Registry struct {
	reads   map[string]ReadHandler
	writes  map[string]WriteHandler
	journal map[string]JournalHandler
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		reads:   map[string]ReadHandler{},
		writes:  map[string]WriteHandler{},
		journal: map[string]JournalHandler{},
	}
}

// Register adds the handler of a `read` method or of an `evaluate` method. It
// panics on a second handler for one method, of any shape: that is a wiring
// mistake found at start, not a condition to handle.
func (registry *Registry) Register(method string, handler ReadHandler) {
	registry.claim(method)
	registry.reads[method] = handler
}

// RegisterWrite adds the handler of a `write` method that records what it
// changes through the recorder it is given.
func (registry *Registry) RegisterWrite(method string, handler WriteHandler) {
	registry.claim(method)
	registry.writes[method] = handler
}

// RegisterJournal adds the handler of a `write` method that acts on changes
// that are already committed.
func (registry *Registry) RegisterJournal(method string, handler JournalHandler) {
	registry.claim(method)
	registry.journal[method] = handler
}

func (registry *Registry) claim(method string) {
	if _, taken := registry.lookup(method); taken {
		panic(fmt.Sprintf("api: method %s registered twice", method))
	}
}

// Names lists the registered methods in order, whatever their shape.
func (registry *Registry) Names() []string {
	names := make([]string, 0, len(registry.reads)+len(registry.writes)+len(registry.journal))
	for name := range registry.reads {
		names = append(names, name)
	}
	for name := range registry.writes {
		names = append(names, name)
	}
	for name := range registry.journal {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// registration is what a method is registered with: exactly one field is set.
type registration struct {
	read    ReadHandler
	write   WriteHandler
	journal JournalHandler
}

// serves says whether a handler of this shape can answer a method of the class.
func (found registration) serves(class contract.Class) bool {
	switch {
	case found.read != nil:
		return class == contract.ClassRead || class == contract.ClassEvaluate
	case found.write != nil, found.journal != nil:
		return class == contract.ClassWrite
	}
	return false
}

func (registry *Registry) lookup(method string) (registration, bool) {
	if read, found := registry.reads[method]; found {
		return registration{read: read}, true
	}
	if write, found := registry.writes[method]; found {
		return registration{write: write}, true
	}
	if journal, found := registry.journal[method]; found {
		return registration{journal: journal}, true
	}
	return registration{}, false
}

// domainRegistrations has one line per domain. A domain's handlers live in a
// file of their own with a function `func registerX(*Registry)` that calls
// Register once per method, so two domains never edit the same lines.
var domainRegistrations = []func(*Registry){
	registerShell,
	registerAccounts,
	registerTransactions,
	registerCard,
	registerPots,
	registerNetWorth,
	registerHome,
	registerBudget,
	registerSubscriptions,
	registerReview,
	registerClosing,
	registerSettings,
	registerChanges,
	registerSettingsWrites,
	registerPotSettingsWrites,
	registerTransactionLineWrites,
}

// NewReadRegistry returns the registry every domain's handlers are in.
func NewReadRegistry() *Registry {
	registry := NewRegistry()
	for _, register := range domainRegistrations {
		register(registry)
	}
	return registry
}
