package suggest

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/authored"
	"github.com/gabrielassisxyz/finwise/internal/brday"
	"github.com/gabrielassisxyz/finwise/internal/money"
)

const (
	// MaxExamples is the maximum number of past person decisions sent per request.
	MaxExamples = 100
	// MaxPurchasesPerRequest caps how many purchases one chat request asks about.
	MaxPurchasesPerRequest = 40
)

// SystemPrompt instructs the model on the exact JSON answer shape. It is a
// package constant in English so every caller and benchmark sends the same one.
const SystemPrompt = `You classify personal bank and credit card purchases in Brazil into one of the user's budget lines.

The user message is a JSON object with:
- "lines": the allowed budget line names for the month.
- "examples": past decisions made by the user, each with an establishment name and the budget line they chose.
- "purchases": purchases to classify, each with a request number "n", a bank description, an amount in BRL, and a date.

Reply with a single JSON object and nothing else:
{"suggestions":[{"n":1,"line":"<exact name from lines>","confidence":85,"justification":"<short reason in Brazilian Portuguese, at most 200 characters>"}]}

Rules:
- Include one entry per purchase, using its exact "n".
- "line" must match one of the names in "lines" exactly.
- "confidence" must be an integer from 0 to 100.
- "justification" must be in Brazilian Portuguese, concise, and at most 200 characters.
- If you cannot decide for a purchase, omit it from "suggestions".`

// ChatRequest is the OpenAI-compatible chat completions request body.
type ChatRequest struct {
	Model       string        `json:"model"`
	Temperature int           `json:"temperature"`
	Messages    []ChatMessage `json:"messages"`
}

// ChatMessage is one message of ChatRequest.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// UserPayload is the JSON content of the user message sent to the model.
type UserPayload struct {
	Lines     []string          `json:"lines"`
	Examples  []PayloadExample  `json:"examples"`
	Purchases []PayloadPurchase `json:"purchases"`
}

// PayloadExample is one scrubbed example in UserPayload.
type PayloadExample struct {
	Establishment string `json:"establishment"`
	Line          string `json:"line"`
}

// PayloadPurchase is one scrubbed purchase in UserPayload. N is local to the
// request so no ledger id leaves the machine.
type PayloadPurchase struct {
	N           int    `json:"n"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
	Day         string `json:"day"`
}

// RequestPurchase links a request-local number n back to its transaction key.
type RequestPurchase struct {
	N   int
	Key authored.TxKey
}

// Request is one ready-to-send chat request for up to MaxPurchasesPerRequest
// purchases of one month, with the lookup tables needed to validate the reply.
type Request struct {
	Month     brday.Month
	Body      []byte
	Purchases []RequestPurchase
	LineIDs   map[string]authored.LineID
}

type scrubbedExample struct {
	establishment string
	lineID        authored.LineID
	fallbackLine  string
}

// BuildRequests turns suggestion candidates into one or more requests per
// month, each holding at most MaxPurchasesPerRequest purchases, with every
// ledger text scrubbed and at most MaxExamples scrubbed examples.
func BuildRequests(model string, candidates api.SuggestionCandidates) ([]Request, error) {
	examples := scrubExamples(candidates.Examples)
	var built []Request
	for _, month := range candidates.Months {
		if len(month.Lines) == 0 || len(month.Purchases) == 0 {
			continue
		}
		lineNames := make([]string, len(month.Lines))
		lineIDs := make(map[string]authored.LineID, len(month.Lines))
		lineByID := make(map[authored.LineID]string, len(month.Lines))
		for i, line := range month.Lines {
			lineNames[i] = line.Name
			lineIDs[line.Name] = line.ID
			lineByID[line.ID] = line.Name
		}
		monthExamples := make([]PayloadExample, 0, len(examples))
		for _, example := range examples {
			lineName := example.fallbackLine
			if current, ok := lineByID[example.lineID]; ok {
				lineName = current
			}
			monthExamples = append(monthExamples, PayloadExample{
				Establishment: example.establishment,
				Line:          lineName,
			})
		}
		for start := 0; start < len(month.Purchases); start += MaxPurchasesPerRequest {
			end := min(start+MaxPurchasesPerRequest, len(month.Purchases))
			chunk := month.Purchases[start:end]
			req, err := buildOneRequest(model, month.Month, lineNames, lineIDs, monthExamples, chunk)
			if err != nil {
				return nil, err
			}
			built = append(built, req)
		}
	}
	return built, nil
}

func scrubExamples(examples []api.SuggestionExample) []scrubbedExample {
	kept := make([]scrubbedExample, 0, min(len(examples), MaxExamples))
	for _, example := range examples {
		scrubbed := strings.TrimSpace(ScrubText(example.Establishment))
		if IsEmptyExampleName(scrubbed) {
			continue
		}
		kept = append(kept, scrubbedExample{
			establishment: scrubbed,
			lineID:        example.LineID,
			fallbackLine:  example.Line,
		})
		if len(kept) == MaxExamples {
			break
		}
	}
	return kept
}

func buildOneRequest(
	model string,
	month brday.Month,
	lineNames []string,
	lineIDs map[string]authored.LineID,
	examples []PayloadExample,
	chunk []api.SuggestionPurchase,
) (Request, error) {
	payloadPurchases := make([]PayloadPurchase, len(chunk))
	requestPurchases := make([]RequestPurchase, len(chunk))
	for i, purchase := range chunk {
		n := i + 1
		payloadPurchases[i] = PayloadPurchase{
			N:           n,
			Description: ScrubText(purchase.Description),
			Amount:      positiveReais(purchase.Cents),
			Day:         string(purchase.Day),
		}
		requestPurchases[i] = RequestPurchase{N: n, Key: purchase.Key}
	}
	userBytes, err := marshalCompact(UserPayload{
		Lines:     lineNames,
		Examples:  examples,
		Purchases: payloadPurchases,
	})
	if err != nil {
		return Request{}, err
	}
	body, err := marshalCompact(ChatRequest{
		Model:       model,
		Temperature: 0,
		Messages: []ChatMessage{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: string(userBytes)},
		},
	})
	if err != nil {
		return Request{}, err
	}
	return Request{
		Month:     month,
		Body:      body,
		Purchases: requestPurchases,
		LineIDs:   lineIDs,
	}, nil
}

func marshalCompact(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// positiveReais formats integer cents as a positive decimal reais string with
// two places (e.g. -1290 -> "12.90"), without touching a float.
func positiveReais(cents money.Cents) string {
	magnitude := uint64(cents)
	if cents < 0 {
		magnitude = -magnitude
	}
	fraction := strconv.FormatUint(magnitude%100, 10)
	if len(fraction) < 2 {
		fraction = "0" + fraction
	}
	return strconv.FormatUint(magnitude/100, 10) + "." + fraction
}
