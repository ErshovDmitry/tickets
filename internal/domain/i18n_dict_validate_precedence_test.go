package domain

import (
	"encoding/json"
	"testing"
)

// TestValidateDictDetailsFormat pins the two detailsFormat-specific checks
// that run after its non-empty check.
func TestValidateDictDetailsFormat(t *testing.T) {
	t.Run("no line break", func(t *testing.T) {
		dj := validDict(t)
		dj.DetailsFormat = "no line break"

		_, err := validateDictJSON(dj)
		if err == nil {
			t.Fatalf("validateDictJSON: expected error, got nil")
		}
		want := "detailsFormat: expected a line break"
		if err.Error() != want {
			t.Errorf("error: got %q, want %q", err.Error(), want)
		}
	})

	t.Run("unexpected placeholder", func(t *testing.T) {
		dj := validDict(t)
		dj.DetailsFormat = "line one\nwith %s placeholder"

		_, err := validateDictJSON(dj)
		if err == nil {
			t.Fatalf("validateDictJSON: expected error, got nil")
		}
		want := "detailsFormat: unexpected placeholder"
		if err.Error() != want {
			t.Errorf("error: got %q, want %q", err.Error(), want)
		}
	})
}

// TestValidateDictPrecedence pins the fixed validation order: when several
// fields are broken, the error of the earliest check in validateDictJSON wins.
func TestValidateDictPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*dictJSON)
		wantErr string
	}{
		{
			"earlier empty check wins",
			func(dj *dictJSON) {
				dj.StatusFmt = ""
				dj.DetailsStub = ""
			},
			"statusFmt: empty",
		},
		{
			"errTicketAlreadyStatus checks %d before %s",
			func(dj *dictJSON) {
				dj.ErrTicketAlreadyStatus = "d=%d%d s=%s%s"
			},
			"errTicketAlreadyStatus: expected 1 %d, got 2",
		},
		{
			"errDetailsFileRead checks %s before %v",
			func(dj *dictJSON) {
				dj.ErrDetailsFileRead = "s=%s%s v=%v%v"
			},
			"errDetailsFileRead: expected 1 %s, got 2",
		},
		{
			"empty phase before placeholder phase",
			func(dj *dictJSON) {
				dj.StatusFmt = "%s%s"
				dj.PriorityFmt = ""
			},
			"priorityFmt: empty",
		},
		{
			"statusFmt count before later empty check",
			func(dj *dictJSON) {
				dj.StatusFmt = "%s%s"
				dj.ErrNoClosedTicketsToArchive = ""
			},
			"statusFmt: expected 1 %s, got 2",
		},
		{
			"createdFmt count before journalCreation count",
			func(dj *dictJSON) {
				dj.CreatedFmt = "- Created: %s"
				dj.JournalCreation = "- %s — ticket created."
			},
			"createdFmt: expected 2 %s, got 1",
		},
		{
			"headers before string fields",
			func(dj *dictJSON) {
				dj.Headers = json.RawMessage(`["a","b","c","d"]`)
				dj.StatusFmt = "%s%s"
			},
			"headers: expected 5 elements, got 4",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dj := validDict(t)
			tc.mutate(&dj)

			_, err := validateDictJSON(dj)
			if err == nil {
				t.Fatalf("validateDictJSON: expected error, got nil")
			}
			if err.Error() != tc.wantErr {
				t.Errorf("error: got %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}
