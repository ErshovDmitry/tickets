package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// validDict returns the embedded English dictionary decoded into dictJSON.
// It is the shared baseline for all validateDictJSON characterization tests.
func validDict(t *testing.T) dictJSON {
	t.Helper()
	raw, err := langsFS.ReadFile("langs/en.json")
	if err != nil {
		t.Fatalf("read embedded en.json: %v", err)
	}
	var dj dictJSON
	if err := json.Unmarshal(raw, &dj); err != nil {
		t.Fatalf("unmarshal embedded en.json: %v", err)
	}
	return dj
}

// stringFieldSetters maps every json key of a string field in dictJSON to a
// pointer into dj. It is the single source of truth for "exactly one field
// broken" test cases; the reflect test below guards its completeness.
func stringFieldSetters(dj *dictJSON) map[string]*string {
	return map[string]*string{
		"statusFmt":                   &dj.StatusFmt,
		"priorityFmt":                 &dj.PriorityFmt,
		"projectFmt":                  &dj.ProjectFmt,
		"createdFmt":                  &dj.CreatedFmt,
		"detailsStub":                 &dj.DetailsStub,
		"detailsFormat":               &dj.DetailsFormat,
		"userCommentsStub":            &dj.UserCommentsStub,
		"journalCreation":             &dj.JournalCreation,
		"journalTransition":           &dj.JournalTransition,
		"journalArchive":              &dj.JournalArchive,
		"warnNewProject":              &dj.WarnNewProject,
		"noTickets":                   &dj.NoTickets,
		"errTitleCTL":                 &dj.ErrTitleCTL,
		"errJournalNewline":           &dj.ErrJournalNewline,
		"errTicketArgNonNumeric":      &dj.ErrTicketArgNonNumeric,
		"errUnknownFlag":              &dj.ErrUnknownFlag,
		"errNoClosedTicketsToArchive": &dj.ErrNoClosedTicketsToArchive,
		"errAlreadyArchived":          &dj.ErrAlreadyArchived,
		"errArchiveOnlyDoneClosed":    &dj.ErrArchiveOnlyDoneClosed,
		"errFileAlreadyExists":        &dj.ErrFileAlreadyExists,
		"msgInitialized":              &dj.MsgInitialized,
		"errInitError":                &dj.ErrInitError,
		"errConflict":                 &dj.ErrConflict,
		"errFlagPRequiresValue":       &dj.ErrFlagPRequiresValue,
		"errFlagPRequiresNonEmpty":    &dj.ErrFlagPRequiresNonEmpty,
		"errRepeatedFlagJson":         &dj.ErrRepeatedFlagJson,
		"errUnknownFlagList":          &dj.ErrUnknownFlagList,
		"errListOneArgMax":            &dj.ErrListOneArgMax,
		"errListFilterInvalid":        &dj.ErrListFilterInvalid,
		"errTitleStartsWithDash":      &dj.ErrTitleStartsWithDash,
		"errTitleEmpty":               &dj.ErrTitleEmpty,
		"errTypeInvalid":              &dj.ErrTypeInvalid,
		"errPriorityInvalid":          &dj.ErrPriorityInvalid,
		"warnTicketsSavedTo":          &dj.WarnTicketsSavedTo,
		"errUnknownArgument":          &dj.ErrUnknownArgument,
		"errFlagRequiresValue":        &dj.ErrFlagRequiresValue,
		"errStatusInvalid":            &dj.ErrStatusInvalid,
		"errTicketAlreadyStatus":      &dj.ErrTicketAlreadyStatus,
		"errDetailsAtBare":            &dj.ErrDetailsAtBare,
		"errDetailsFileRead":          &dj.ErrDetailsFileRead,
		"errDetailsStdinRead":         &dj.ErrDetailsStdinRead,
		"warnExtraArgsIgnored":        &dj.WarnExtraArgsIgnored,
		"errTicketNotFound":           &dj.ErrTicketNotFound,
		"hintUseList":                 &dj.HintUseList,
		"hintNumbersStartAtOne":       &dj.HintNumbersStartAtOne,
		"hintStoreEmpty":              &dj.HintStoreEmpty,
		"hintNumbersUpTo":             &dj.HintNumbersUpTo,
		"errAmbiguousNumber":          &dj.ErrAmbiguousNumber,
		"warnAmbiguousNumber":         &dj.WarnAmbiguousNumber,
		"warnAmbiguousNumberSkipped":  &dj.WarnAmbiguousNumberSkipped,
		"updateCheckUpToDate":         &dj.UpdateCheckUpToDate,
		"updateCheckAvailable":        &dj.UpdateCheckAvailable,
		"updateCheckNoReleases":       &dj.UpdateCheckNoReleases,
		"updateCheckDev":              &dj.UpdateCheckDev,
		"errUpdateCheck":              &dj.ErrUpdateCheck,
	}
}

// TestValidateDictStringFieldsCovered guards the setter table: dictJSON must
// have exactly 55 string fields, every one covered by stringFieldSetters,
// and each setter must point at the struct field carrying the same json tag.
// A newly added string field without a test case fails here.
func TestValidateDictStringFieldsCovered(t *testing.T) {
	dj := validDict(t)
	setters := stringFieldSetters(&dj)

	typ := reflect.TypeOf(dictJSON{})
	val := reflect.ValueOf(&dj).Elem()

	stringFields := 0
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.String {
			continue
		}
		stringFields++
		tag := field.Tag.Get("json")
		sp, ok := setters[tag]
		if !ok {
			t.Errorf("string field %s (json %q) has no setter entry", field.Name, tag)
			continue
		}
		const sentinel = "\x00coverage-probe"
		*sp = sentinel
		if got := val.Field(i).String(); got != sentinel {
			t.Errorf("setter for json %q points at wrong field (json tag mismatch)", tag)
		}
		*sp = ""
	}
	if stringFields != 55 {
		t.Errorf("dictJSON: expected 55 string fields, got %d", stringFields)
	}
	if len(setters) != stringFields {
		t.Errorf("setter table: expected %d entries, got %d", stringFields, len(setters))
	}
}

// TestValidateDictBaseline pins the happy path: the embedded en.json passes
// validation and returns its five section headers.
func TestValidateDictBaseline(t *testing.T) {
	headers, err := validateDictJSON(validDict(t))
	if err != nil {
		t.Fatalf("validateDictJSON(valid en.json): unexpected error: %v", err)
	}
	want := [5]string{"## Summary", "## Details", "## User comments", "## Comments", "## Journal"}
	if headers != want {
		t.Errorf("headers: got %q, want %q", headers, want)
	}
}

// TestValidateDictHeaders covers the headers block: invalid JSON, wrong
// element count, and an empty element.
func TestValidateDictHeaders(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{"invalid JSON", "[", ""},
		{"four elements", `["a","b","c","d"]`, "headers: expected 5 elements, got 4"},
		{"six elements", `["a","b","c","d","e","f"]`, "headers: expected 5 elements, got 6"},
		{"empty element", `["a","","c","d","e"]`, "headers[1]: empty string not allowed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dj := validDict(t)
			dj.Headers = json.RawMessage(tc.raw)

			_, err := validateDictJSON(dj)
			if err == nil {
				t.Fatalf("validateDictJSON: expected error, got nil")
			}
			if tc.wantErr == "" {
				if !strings.HasPrefix(err.Error(), "headers: ") {
					t.Errorf("error prefix: got %q, want prefix %q", err.Error(), "headers: ")
				}
				var se *json.SyntaxError
				if !errors.As(err, &se) {
					t.Errorf("errors.As: expected *json.SyntaxError, got %T", err)
				}
				return
			}
			if err.Error() != tc.wantErr {
				t.Errorf("error: got %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestValidateDictEmptyFields nulls exactly one string field at a time and
// expects the "<jsonKey>: empty" error for it.
func TestValidateDictEmptyFields(t *testing.T) {
	probe := stringFieldSetters(&dictJSON{})
	if len(probe) != 55 {
		t.Fatalf("setter table: expected 55 entries, got %d", len(probe))
	}
	for key := range probe {
		t.Run(key, func(t *testing.T) {
			dj := validDict(t)
			*stringFieldSetters(&dj)[key] = ""

			_, err := validateDictJSON(dj)
			if err == nil {
				t.Fatalf("validateDictJSON: expected error, got nil")
			}
			want := key + ": empty"
			if err.Error() != want {
				t.Errorf("error: got %q, want %q", err.Error(), want)
			}
		})
	}
}
