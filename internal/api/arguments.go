package api

import (
	"bytes"
	"encoding/json"
)

// decodeArgument reads one argument strictly: a field the method does not know
// is as wrong as a missing one, and a body with anything after the value is not
// one argument.
func decodeArgument(raw json.RawMessage, into any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return &badArgumentsError{}
	}
	if decoder.More() {
		return &badArgumentsError{}
	}
	return nil
}

// stringArgument is the one argument of a method that takes an id.
func stringArgument(args []json.RawMessage) (string, error) {
	if len(args) != 1 {
		return "", &badArgumentsError{}
	}
	var value *string
	if err := decodeArgument(args[0], &value); err != nil || value == nil {
		return "", &badArgumentsError{}
	}
	return *value, nil
}
