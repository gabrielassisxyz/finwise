package api

import (
	"bytes"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

// compileArgumentSchemas compiles the argument schema of every method of the
// manifest, so that one that does not compile stops the program at start
// instead of being half-honoured. Which methods the schemas are then applied to
// is the router's decision.
func compileArgumentSchemas(methods []contract.Method) (map[string]*jsonschema.Schema, error) {
	schemas := make(map[string]*jsonschema.Schema, len(methods))
	for _, method := range methods {
		name := method.Qualified()
		schema, err := compileArgumentSchema(name, method.ArgumentsSchema)
		if err != nil {
			return nil, fmt.Errorf("api: the argument schema of %s does not compile: %w", name, err)
		}
		schemas[name] = schema
	}
	return schemas, nil
}

func compileArgumentSchema(name string, document []byte) (*jsonschema.Schema, error) {
	parsed, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	location := "urn:finwise:arguments:" + name
	if err := compiler.AddResource(location, parsed); err != nil {
		return nil, err
	}
	return compiler.Compile(location)
}

// argumentsMatch applies a method's schema to the bytes of the request's
// argument array. They are decoded by the schema library's own decoder, which
// keeps every number as the text it arrived as, so an amount never passes
// through a float on its way to the schema.
func argumentsMatch(schema *jsonschema.Schema, body []byte) bool {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return false
	}
	return schema.Validate(instance) == nil
}
