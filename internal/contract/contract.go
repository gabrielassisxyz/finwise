// Package contract is the Go side of the interface's API manifest: the list of
// methods, their routes, their classes and their argument schemas, and the
// reasons a refusal may give, read from the generated contract.json.
package contract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
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
	// ArgumentsSchema is a draft-07 document that compiles on its own: the
	// method's argument schema with every definition it refers to.
	ArgumentsSchema json.RawMessage
}

// Qualified is the method as the interface names it, "domain.name".
func (method Method) Qualified() string {
	return method.Domain + "." + method.Name
}

// Manifest is what the server takes from contract.json.
type Manifest struct {
	Methods []Method
	// RefusalReasons are the reasons a refused call may carry, in the
	// contract's order.
	RefusalReasons []string
}

//go:embed contract.json
var manifestJSON []byte

// refusalReasonsDefinition is the name the generator gives the contract's
// RefusalReason union.
const refusalReasonsDefinition = "RefusalReason"

const definitionsRef = "#/definitions/"

type manifestFile struct {
	Methods     []manifestMethod           `json:"methods"`
	Definitions map[string]json.RawMessage `json:"definitions"`
}

type manifestMethod struct {
	Domain    string          `json:"domain"`
	Name      string          `json:"name"`
	Route     string          `json:"route"`
	Class     Class           `json:"class"`
	Arguments json.RawMessage `json:"arguments"`
}

type argumentsSchema struct {
	Items    []json.RawMessage `json:"items"`
	MinItems *int              `json:"minItems"`
	MaxItems *int              `json:"maxItems"`
}

type argumentsDocument struct {
	Schema      string                     `json:"$schema"`
	AllOf       []json.RawMessage          `json:"allOf"`
	Definitions map[string]json.RawMessage `json:"definitions,omitempty"`
}

// Load parses the embedded manifest and returns its methods. It fails on
// anything it does not understand, so a regenerated manifest that changed shape
// stops the program at start instead of being half-honoured.
func Load() ([]Method, error) {
	manifest, err := LoadManifest()
	if err != nil {
		return nil, err
	}
	return manifest.Methods, nil
}

// LoadManifest parses the embedded manifest, failing as Load does.
func LoadManifest() (Manifest, error) {
	return parse(manifestJSON)
}

func parse(raw []byte) (Manifest, error) {
	var file manifestFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return Manifest{}, fmt.Errorf("contract: parse manifest: %w", err)
	}
	if len(file.Methods) == 0 {
		return Manifest{}, fmt.Errorf("contract: manifest lists no methods")
	}
	reasons, err := refusalReasons(file.Definitions)
	if err != nil {
		return Manifest{}, err
	}
	methods := make([]Method, 0, len(file.Methods))
	seen := make(map[string]bool, len(file.Methods))
	for _, entry := range file.Methods {
		method, err := entry.toMethod(file.Definitions)
		if err != nil {
			return Manifest{}, err
		}
		if seen[method.Route] {
			return Manifest{}, fmt.Errorf("contract: route %s listed twice", method.Route)
		}
		seen[method.Route] = true
		methods = append(methods, method)
	}
	return Manifest{Methods: methods, RefusalReasons: reasons}, nil
}

func (entry manifestMethod) toMethod(definitions map[string]json.RawMessage) (Method, error) {
	label := entry.Domain + "." + entry.Name
	switch entry.Class {
	case ClassSession, ClassRead, ClassEvaluate, ClassWrite:
	default:
		return Method{}, fmt.Errorf("contract: %s has unknown class %q", label, entry.Class)
	}
	wantRoute := "/api/" + entry.Domain + "/" + entry.Name
	if entry.Route != wantRoute {
		return Method{}, fmt.Errorf("contract: %s has route %q, want %q", label, entry.Route, wantRoute)
	}
	if len(entry.Arguments) == 0 || bytes.Equal(entry.Arguments, []byte("null")) {
		return Method{}, fmt.Errorf("contract: %s has no argument schema", label)
	}
	var arguments argumentsSchema
	if err := json.Unmarshal(entry.Arguments, &arguments); err != nil {
		return Method{}, fmt.Errorf("contract: %s argument schema: %w", label, err)
	}
	maxArgs := len(arguments.Items)
	if arguments.MaxItems != nil {
		maxArgs = *arguments.MaxItems
	}
	minArgs := maxArgs
	if arguments.MinItems != nil {
		minArgs = *arguments.MinItems
	}
	if minArgs > maxArgs {
		return Method{}, fmt.Errorf("contract: %s has minItems above maxItems", label)
	}
	document, err := selfContained(entry.Arguments, definitions)
	if err != nil {
		return Method{}, fmt.Errorf("contract: %s argument schema: %w", label, err)
	}
	return Method{
		Domain: entry.Domain, Name: entry.Name, Route: entry.Route, Class: entry.Class,
		MinArguments: minArgs, MaxArguments: maxArgs, ArgumentsSchema: document,
	}, nil
}

// selfContained wraps a schema in a document that carries the definitions it
// reaches, directly or through other definitions, so that its `$ref`s resolve
// against the document's root exactly as they do against the manifest's.
func selfContained(schema json.RawMessage, definitions map[string]json.RawMessage) (json.RawMessage, error) {
	reached := map[string]json.RawMessage{}
	pending := []json.RawMessage{schema}
	for len(pending) > 0 {
		next := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		names, err := referencedDefinitions(next)
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			if _, done := reached[name]; done {
				continue
			}
			body, ok := definitions[name]
			if !ok {
				return nil, fmt.Errorf("$ref to missing definition %q", name)
			}
			reached[name] = body
			pending = append(pending, body)
		}
	}
	return json.Marshal(argumentsDocument{
		Schema:      "http://json-schema.org/draft-07/schema#",
		AllOf:       []json.RawMessage{schema},
		Definitions: reached,
	})
}

// referencedDefinitions lists the definition names a schema's `$ref`s name,
// decoded from the percent-encoding the generator writes them in.
func referencedDefinitions(schema json.RawMessage) ([]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(schema))
	decoder.UseNumber()
	var tree any
	if err := decoder.Decode(&tree); err != nil {
		return nil, err
	}
	var names []string
	var walkErr error
	var walk func(node any)
	walk = func(node any) {
		switch value := node.(type) {
		case []any:
			for _, child := range value {
				walk(child)
			}
		case map[string]any:
			for key, child := range value {
				ref, isRef := child.(string)
				if key != "$ref" || !isRef {
					walk(child)
					continue
				}
				name, err := definitionName(ref)
				if err != nil && walkErr == nil {
					walkErr = err
				}
				names = append(names, name)
			}
		}
	}
	walk(tree)
	return names, walkErr
}

func definitionName(ref string) (string, error) {
	encoded, ok := strings.CutPrefix(ref, definitionsRef)
	if !ok || strings.Contains(encoded, "/") {
		return "", fmt.Errorf("$ref %q does not name a definition", ref)
	}
	return url.PathUnescape(encoded)
}

func refusalReasons(definitions map[string]json.RawMessage) ([]string, error) {
	raw, ok := definitions[refusalReasonsDefinition]
	if !ok {
		return nil, fmt.Errorf("contract: manifest has no %s definition", refusalReasonsDefinition)
	}
	var list struct {
		Type string   `json:"type"`
		Enum []string `json:"enum"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("contract: %s: %w", refusalReasonsDefinition, err)
	}
	if list.Type != "string" || len(list.Enum) == 0 {
		return nil, fmt.Errorf("contract: %s is not a list of strings", refusalReasonsDefinition)
	}
	seen := make(map[string]bool, len(list.Enum))
	for _, reason := range list.Enum {
		if reason == "" || seen[reason] {
			return nil, fmt.Errorf("contract: %s has an empty or repeated reason", refusalReasonsDefinition)
		}
		seen[reason] = true
	}
	return list.Enum, nil
}
