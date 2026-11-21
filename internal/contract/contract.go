// Package contract is the Go side of the interface's API manifest: the list of
// methods, their routes and their classes, read from the generated contract.json.
package contract

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Class says what a method is allowed to do, which is what the router decides on.
type Class string

const (
	ClassSession  Class = "session"
	ClassRead     Class = "read"
	ClassEvaluate Class = "evaluate"
	ClassWrite    Class = "write"
)

// Method is one entry of the manifest.
type Method struct {
	Domain string
	Name   string
	Route  string
	Class  Class
	// MinArguments and MaxArguments bound the length of the JSON argument array.
	MinArguments int
	MaxArguments int
}

// Qualified is the method as the interface names it, "domain.name".
func (method Method) Qualified() string {
	return method.Domain + "." + method.Name
}

//go:embed contract.json
var manifestJSON []byte

type manifestFile struct {
	Methods []manifestMethod `json:"methods"`
}

type manifestMethod struct {
	Domain    string          `json:"domain"`
	Name      string          `json:"name"`
	Route     string          `json:"route"`
	Class     Class           `json:"class"`
	Arguments argumentsSchema `json:"arguments"`
}

type argumentsSchema struct {
	Items    []json.RawMessage `json:"items"`
	MinItems *int              `json:"minItems"`
	MaxItems *int              `json:"maxItems"`
}

// Load parses the embedded manifest. It fails on anything it does not
// understand, so a regenerated manifest that changed shape stops the program
// at start instead of being half-honoured.
func Load() ([]Method, error) {
	return parse(manifestJSON)
}

func parse(raw []byte) ([]Method, error) {
	var file manifestFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("contract: parse manifest: %w", err)
	}
	if len(file.Methods) == 0 {
		return nil, fmt.Errorf("contract: manifest lists no methods")
	}
	methods := make([]Method, 0, len(file.Methods))
	seen := make(map[string]bool, len(file.Methods))
	for _, entry := range file.Methods {
		method, err := entry.toMethod()
		if err != nil {
			return nil, err
		}
		if seen[method.Route] {
			return nil, fmt.Errorf("contract: route %s listed twice", method.Route)
		}
		seen[method.Route] = true
		methods = append(methods, method)
	}
	return methods, nil
}

func (entry manifestMethod) toMethod() (Method, error) {
	switch entry.Class {
	case ClassSession, ClassRead, ClassEvaluate, ClassWrite:
	default:
		return Method{}, fmt.Errorf("contract: %s.%s has unknown class %q", entry.Domain, entry.Name, entry.Class)
	}
	wantRoute := "/api/" + entry.Domain + "/" + entry.Name
	if entry.Route != wantRoute {
		return Method{}, fmt.Errorf("contract: %s.%s has route %q, want %q", entry.Domain, entry.Name, entry.Route, wantRoute)
	}
	maxArgs := len(entry.Arguments.Items)
	if entry.Arguments.MaxItems != nil {
		maxArgs = *entry.Arguments.MaxItems
	}
	minArgs := maxArgs
	if entry.Arguments.MinItems != nil {
		minArgs = *entry.Arguments.MinItems
	}
	if minArgs > maxArgs {
		return Method{}, fmt.Errorf("contract: %s.%s has minItems above maxItems", entry.Domain, entry.Name)
	}
	return Method{
		Domain: entry.Domain, Name: entry.Name, Route: entry.Route, Class: entry.Class,
		MinArguments: minArgs, MaxArguments: maxArgs,
	}, nil
}
