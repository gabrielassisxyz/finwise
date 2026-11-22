package authored

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Value is one column's content in a journaled table: null, an integer or a
// text. Amounts are integer cents and travel as integers; a REAL or a BLOB
// column cannot be journaled, so no number of the journal passes through a
// float.
type Value struct {
	kind    valueKind
	integer int64
	text    string
}

type valueKind int

const (
	valueNull valueKind = iota
	valueInteger
	valueText
)

// Null is the SQL null.
var Null = Value{}

// Integer is an integer column's content; money.Cents converts to it as is.
func Integer(n int64) Value { return Value{kind: valueInteger, integer: n} }

// Text is a text column's content.
func Text(s string) Value { return Value{kind: valueText, text: s} }

// IsNull says whether the value is the SQL null.
func (value Value) IsNull() bool { return value.kind == valueNull }

// Int64 is the integer the value holds, if it holds one.
func (value Value) Int64() (int64, bool) { return value.integer, value.kind == valueInteger }

// Str is the text the value holds, if it holds one.
func (value Value) Str() (string, bool) { return value.text, value.kind == valueText }

// argument is the value as database/sql takes it.
func (value Value) argument() any {
	switch value.kind {
	case valueInteger:
		return value.integer
	case valueText:
		return value.text
	}
	return nil
}

// scannedValue turns what the driver returned into a Value. Anything that is
// not null, an integer or valid UTF-8 text is refused instead of being turned
// into something else, which is where a REAL column would otherwise lose
// digits.
func scannedValue(scanned any) (Value, error) {
	switch typed := scanned.(type) {
	case nil:
		return Null, nil
	case int64:
		return Integer(typed), nil
	case string:
		if !utf8.ValidString(typed) {
			return Value{}, errors.New("authored: a journaled text is not valid UTF-8")
		}
		return Text(typed), nil
	}
	return Value{}, fmt.Errorf("authored: a journaled column holds a %T, which the journal does not carry", scanned)
}

// canonical is the value's JSON. The form is written by hand, not by
// encoding/json, so that a value journaled today renders the same text when it
// is compared years from now, whatever a library's escaping becomes.
func (value Value) canonical() string {
	switch value.kind {
	case valueInteger:
		return strconv.FormatInt(value.integer, 10)
	case valueText:
		return quoteJSON(value.text)
	}
	return "null"
}

// quoteJSON escapes only what JSON requires: the quote, the backslash and the
// control characters, the last always as \u00xx.
func quoteJSON(text string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range text {
		switch {
		case r == '"':
			out.WriteString(`\"`)
		case r == '\\':
			out.WriteString(`\\`)
		case r < 0x20:
			fmt.Fprintf(&out, `\u%04x`, r)
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
	return out.String()
}

// EncodeKey is a row's key as the journal writes it: a JSON array of the key
// columns' values in key order.
func EncodeKey(values ...Value) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = value.canonical()
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// Row is a row of a journaled table, by column name.
type Row map[string]Value

// encodeRow is a row as a JSON object whose members are the table's columns in
// the table's column order.
func encodeRow(table *Table, row Row) string {
	parts := make([]string, len(table.Columns))
	for i, column := range table.Columns {
		parts[i] = quoteJSON(column.Name) + ":" + row[column.Name].canonical()
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// decodeValue reads one canonical value. Numbers are read as json.Number and
// parsed as integers: decoded into an interface they would become float64 and
// lose every digit past 2^53.
func decodeValue(text string) (Value, error) {
	decoder := newDecoder(text)
	value, err := nextValue(decoder)
	if err != nil {
		return Value{}, err
	}
	if err := expectEnd(decoder); err != nil {
		return Value{}, err
	}
	if value.canonical() != text {
		return Value{}, fmt.Errorf("authored: %d bytes of journal are not a canonical value", len(text))
	}
	return value, nil
}

func decodeKey(text string) ([]Value, error) {
	decoder := newDecoder(text)
	if err := expectDelimiter(decoder, '['); err != nil {
		return nil, err
	}
	var values []Value
	for decoder.More() {
		value, err := nextValue(decoder)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := expectDelimiter(decoder, ']'); err != nil {
		return nil, err
	}
	if err := expectEnd(decoder); err != nil {
		return nil, err
	}
	if EncodeKey(values...) != text {
		return nil, errors.New("authored: a journaled key is not canonical")
	}
	return values, nil
}

// decodeRow reads a row of table, which must name every column of it once and
// nothing else.
func decodeRow(table *Table, text string) (Row, error) {
	decoder := newDecoder(text)
	if err := expectDelimiter(decoder, '{'); err != nil {
		return nil, err
	}
	row := Row{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, errors.New("authored: a journaled row has a member that is not named")
		}
		value, err := nextValue(decoder)
		if err != nil {
			return nil, err
		}
		row[name] = value
	}
	if err := expectDelimiter(decoder, '}'); err != nil {
		return nil, err
	}
	if err := expectEnd(decoder); err != nil {
		return nil, err
	}
	if len(row) != len(table.Columns) || encodeRow(table, row) != text {
		return nil, fmt.Errorf("authored: a journaled row of %s does not hold exactly its columns", table.Name)
	}
	return row, nil
}

func newDecoder(text string) *json.Decoder {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	return decoder
}

func nextValue(decoder *json.Decoder) (Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return Value{}, err
	}
	switch typed := token.(type) {
	case nil:
		return Null, nil
	case string:
		return Text(typed), nil
	case json.Number:
		n, err := strconv.ParseInt(typed.String(), 10, 64)
		if err != nil {
			return Value{}, errors.New("authored: a journaled number is not an integer")
		}
		return Integer(n), nil
	}
	return Value{}, errors.New("authored: the journal holds a value that is neither null, an integer nor a text")
}

func expectDelimiter(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != want {
		return fmt.Errorf("authored: the journal holds %v where %v was expected", token, want)
	}
	return nil
}

func expectEnd(decoder *json.Decoder) error {
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("authored: the journal holds more than one value where one was expected")
	}
	return nil
}
