package suggest

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gabrielassisxyz/finwise/internal/authored"
)

const (
	// MaxJustificationRunes is the maximum trimmed character length accepted for
	// a model's justification.
	MaxJustificationRunes = 200
)

var errMalformedResponse = errors.New("suggest: malformed model response")

// ParsedOutcome is the validated result of one model response: one row per
// purchase in the request (usable or nil-line unusable) and their counts.
type ParsedOutcome struct {
	Rows      []authored.ModelSuggestionInput
	Suggested int
	Unusable  int
}

type chatCompletionEnvelope struct {
	Choices []struct {
		Message struct {
			Content *string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type acceptedEntry struct {
	line          authored.LineID
	confidence    int64
	justification string
}

// ParseResponse validates an untrusted HTTP response body against request.
// If the envelope or message content is not a single JSON object with a
// "suggestions" array (optionally inside a fenced code block), it returns an
// error so the request fails as malformed. Otherwise each entry is validated
// independently; invalid, repeated, unknown or omitted entries produce a row
// with no line so the purchase is marked asked and unusable.
func ParseResponse(raw []byte, request Request) (ParsedOutcome, error) {
	var envelope chatCompletionEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return ParsedOutcome{}, errMalformedResponse
	}
	if len(envelope.Choices) == 0 || envelope.Choices[0].Message.Content == nil {
		return ParsedOutcome{}, errMalformedResponse
	}
	content, ok := unwrapFencedBlock(*envelope.Choices[0].Message.Content)
	if !ok {
		return ParsedOutcome{}, errMalformedResponse
	}
	entries, ok := decodeSuggestionsArray(content)
	if !ok {
		return ParsedOutcome{}, errMalformedResponse
	}

	nCounts := map[int]int{}
	for _, item := range entries {
		fields, ok := decodeJSONObject(item)
		if !ok {
			continue
		}
		if n, valid := parseStrictInt(fields["n"]); valid && n >= 1 && n <= int64(len(request.Purchases)) {
			nCounts[int(n)]++
		}
	}

	accepted := map[int]acceptedEntry{}
	for _, item := range entries {
		n, entry, ok := validateEntry(item, request, nCounts)
		if !ok {
			continue
		}
		accepted[n] = entry
	}

	rows := make([]authored.ModelSuggestionInput, len(request.Purchases))
	outcome := ParsedOutcome{Rows: rows}
	for i, purchase := range request.Purchases {
		entry, ok := accepted[purchase.N]
		if !ok {
			rows[i] = authored.ModelSuggestionInput{Key: purchase.Key}
			outcome.Unusable++
			continue
		}
		line := entry.line
		confidence := entry.confidence
		justification := entry.justification
		rows[i] = authored.ModelSuggestionInput{
			Key:           purchase.Key,
			Line:          &line,
			Confidence:    &confidence,
			Justification: &justification,
		}
		outcome.Suggested++
	}
	return outcome, nil
}

func unwrapFencedBlock(raw string) ([]byte, bool) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") {
		return []byte(trimmed), trimmed != ""
	}
	newline := strings.IndexByte(trimmed, '\n')
	if newline < 0 {
		return nil, false
	}
	body := trimmed[newline+1:]
	if !strings.HasSuffix(body, "```") {
		return nil, false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(body, "```"))
	return []byte(inner), inner != ""
}

func decodeSuggestionsArray(content []byte) ([]json.RawMessage, bool) {
	fields, ok := decodeJSONObject(content)
	if !ok {
		return nil, false
	}
	rawSuggestions, present := fields["suggestions"]
	if !present {
		return nil, false
	}
	trimmed := bytes.TrimSpace(rawSuggestions)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, false
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(trimmed, &entries); err != nil {
		return nil, false
	}
	return entries, true
}

func decodeJSONObject(raw []byte) (map[string]json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, false
	}
	return fields, true
}

func validateEntry(raw json.RawMessage, request Request, nCounts map[int]int) (int, acceptedEntry, bool) {
	fields, ok := decodeJSONObject(raw)
	if !ok {
		return 0, acceptedEntry{}, false
	}
	nVal, ok := parseStrictInt(fields["n"])
	if !ok || nVal < 1 || nVal > int64(len(request.Purchases)) {
		return 0, acceptedEntry{}, false
	}
	n := int(nVal)
	if nCounts[n] != 1 {
		return 0, acceptedEntry{}, false
	}
	lineName, ok := parseJSONString(fields["line"])
	if !ok {
		return 0, acceptedEntry{}, false
	}
	lineID, ok := request.LineIDs[lineName]
	if !ok {
		return 0, acceptedEntry{}, false
	}
	confidence, ok := parseStrictInt(fields["confidence"])
	if !ok || confidence < 0 || confidence > 100 {
		return 0, acceptedEntry{}, false
	}
	justification, ok := parseJSONString(fields["justification"])
	if !ok || !utf8.ValidString(justification) {
		return 0, acceptedEntry{}, false
	}
	trimmed := strings.TrimSpace(justification)
	count := utf8.RuneCountInString(trimmed)
	if count < 1 || count > MaxJustificationRunes {
		return 0, acceptedEntry{}, false
	}
	return n, acceptedEntry{
		line:          lineID,
		confidence:    confidence,
		justification: trimmed,
	}, true
}

// parseStrictInt decodes a JSON integer token directly from bytes without
// touching a float. Fractions (82.5, 82.0), exponents, strings and nulls are
// rejected.
func parseStrictInt(raw json.RawMessage) (int64, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0, false
	}
	digits := trimmed
	if digits[0] == '-' {
		digits = digits[1:]
		if len(digits) == 0 {
			return 0, false
		}
	}
	if len(digits) > 1 && digits[0] == '0' {
		return 0, false
	}
	for _, b := range digits {
		if b < '0' || b > '9' {
			return 0, false
		}
	}
	value, err := strconv.ParseInt(string(trimmed), 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func parseJSONString(raw json.RawMessage) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return "", false
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return "", false
	}
	return value, true
}
