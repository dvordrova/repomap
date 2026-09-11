package deepseek

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// These are explicit provider context refusals. A generic 400, quota code or
// mention of a context setting is not authority to split a semantic request.
// Two wordings carry the same four numbers: DeepSeek's "This model's maximum
// context length is L tokens. However, you requested R tokens (I in the
// messages, O in the completion)" and a gateway's "Requested token count
// exceeds the model's maximum context length of L tokens. You requested a
// total of R tokens: I tokens from the input messages and O tokens for the
// completion". Both are explicit refusals with consistent counts.
var numericContextLimit = regexp.MustCompile(`(?i)maximum context length (?:is|of) ([0-9]+) tokens\.? ?(?:However, )?you requested (?:a total of )?([0-9]+) tokens[:( ]+([0-9]+) (?:tokens )?(?:in|from) the (?:input )?messages,? (?:and )?([0-9]+) (?:tokens )?(?:in|for) the completion`)
var inputTokenLimit = regexp.MustCompile(`(?i)^input token exceed the limit(?: \(request id: [^\r\n]*\))?\.?$`)

// An OpenAI-compatible server in front of the same model family words the
// same refusal as "The input (703005 tokens) is longer than the model's
// context length (524288 tokens)". Only the input is counted there.
var inputLongerThanContext = regexp.MustCompile(`(?i)the input \(([0-9]+) tokens\) is longer than the model'?s context length \(([0-9]+) tokens\)`)

func providerContextLimit(status int, body []byte) *ResourceLimitError {
	if status != http.StatusBadRequest && status != http.StatusRequestEntityTooLarge {
		return nil
	}
	var envelope struct {
		Error   json.RawMessage `json:"error"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	codeRaw, message := envelope.Code, envelope.Message
	if len(envelope.Error) > 0 {
		var detail struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		}
		if err := json.Unmarshal(envelope.Error, &detail); err != nil {
			return nil
		}
		codeRaw, message = detail.Code, detail.Message
	}
	resource := &ResourceLimitError{Kind: ResourceLimitContextTokens, HTTPStatus: status}
	if values := numericContextLimit.FindStringSubmatch(message); values != nil {
		numbers := make([]int, 4)
		for i, raw := range values[1:] {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return nil
			}
			numbers[i] = n
		}
		limit, requested, input, output := numbers[0], numbers[1], numbers[2], numbers[3]
		if limit <= 0 || requested <= limit || input > requested || output != requested-input {
			return nil
		}
		resource.Limit, resource.Observed, resource.ObservedKnown = limit, requested, true
		resource.InputTokens, resource.ConfiguredMaxTokens = input, output
		return resource
	}
	if values := inputLongerThanContext.FindStringSubmatch(message); values != nil {
		input, inputErr := strconv.Atoi(values[1])
		limit, limitErr := strconv.Atoi(values[2])
		if inputErr != nil || limitErr != nil || limit <= 0 || input <= limit {
			return nil
		}
		resource.Limit, resource.Observed, resource.ObservedKnown = limit, input, true
		resource.InputTokens = input
		return resource
	}
	var code string
	_ = json.Unmarshal(codeRaw, &code)
	if code == "context_length_exceeded" || inputTokenLimit.MatchString(strings.TrimSpace(message)) {
		return resource
	}
	return nil
}
