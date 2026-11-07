package domain

import (
	"encoding/json"
	"fmt"
)

// validateDictJSON validates one decoded dictionary in a fixed order and
// returns its parsed section headers. Error text and precedence are part
// of the observable behaviour.
func validateDictJSON(dj dictJSON) ([5]string, error) {
	var rawHeaders []string

	// Parse headers into slice first to validate length
	if err := json.Unmarshal(dj.Headers, &rawHeaders); err != nil {
		return [5]string{}, fmt.Errorf("headers: %w", err)
	}
	if len(rawHeaders) != 5 {
		return [5]string{}, fmt.Errorf("headers: expected 5 elements, got %d", len(rawHeaders))
	}
	for i, h := range rawHeaders {
		if h == "" {
			return [5]string{}, fmt.Errorf("headers[%d]: empty string not allowed", i)
		}
	}
	var headers [5]string
	copy(headers[:], rawHeaders)

	// Field checks run as the ordered rule table dictValidationRules
	// (i18n_dict_rules.go); its entry order is the validation order.
	for _, r := range dictValidationRules {
		if err := r.check(r.value(dj)); err != nil {
			return [5]string{}, err
		}
	}

	return headers, nil
}
