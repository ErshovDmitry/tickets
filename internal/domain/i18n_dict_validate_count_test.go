package domain

import "testing"

// TestValidateDictPlaceholderCounts breaks exactly one placeholder count at
// a time: the field stays non-empty, only the number of %<verb> slots is
// wrong. Error text is pinned byte-for-byte.
func TestValidateDictPlaceholderCounts(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr string
	}{
		{"statusFmt", "%s%s", "statusFmt: expected 1 %s, got 2"},
		{"priorityFmt", "%s%s", "priorityFmt: expected 1 %s, got 2"},
		{"projectFmt", "%s%s", "projectFmt: expected 1 %s, got 2"},
		{"createdFmt", "%s", "createdFmt: expected 2 %s, got 1"},
		{"journalCreation", "%s", "journalCreation: expected 2 %s, got 1"},
		{"journalTransition", "%s%s", "journalTransition: expected 3 %s, got 2"},
		{"journalArchive", "%s", "journalArchive: expected 2 %s, got 1"},
		{"warnNewProject", "%s%s", "warnNewProject: expected 1 %s, got 2"},
		{"noTickets", "%s%s", "noTickets: expected 1 %s, got 2"},
		{"errTicketArgNonNumeric", "%s%s", "errTicketArgNonNumeric: expected 1 %s, got 2"},
		{"errUnknownFlag", "%s%s", "errUnknownFlag: expected 1 %s, got 2"},
		{"errArchiveOnlyDoneClosed", "%s%s", "errArchiveOnlyDoneClosed: expected 1 %s, got 2"},
		{"errFileAlreadyExists", "%s%s", "errFileAlreadyExists: expected 1 %s, got 2"},
		{"msgInitialized", "%s%s", "msgInitialized: expected 1 %s, got 2"},
		{"errInitError", "%v%v", "errInitError: expected 1 %v, got 2"},
		{"errConflict", "%s%s", "errConflict: expected 1 %s, got 2"},
		{"errUnknownFlagList", "%s%s", "errUnknownFlagList: expected 1 %s, got 2"},
		{"warnTicketsSavedTo", "%s%s", "warnTicketsSavedTo: expected 1 %s, got 2"},
		{"errUnknownArgument", "%s%s", "errUnknownArgument: expected 1 %s, got 2"},
		{"errFlagRequiresValue", "%s%s", "errFlagRequiresValue: expected 1 %s, got 2"},
		{"errTicketAlreadyStatus", "s=%s d=%d%d", "errTicketAlreadyStatus: expected 1 %d, got 2"},
		{"errTicketAlreadyStatus", "d=%d s=%s%s", "errTicketAlreadyStatus: expected 1 %s, got 2"},
		{"errDetailsFileRead", "v=%v s=%s%s", "errDetailsFileRead: expected 1 %s, got 2"},
		{"errDetailsFileRead", "s=%s v=%v%v", "errDetailsFileRead: expected 1 %v, got 2"},
		{"errDetailsStdinRead", "%v%v", "errDetailsStdinRead: expected 1 %v, got 2"},
		{"warnExtraArgsIgnored", "%s%s", "warnExtraArgsIgnored: expected 1 %s, got 2"},
		{"errTicketNotFound", "%s%s", "errTicketNotFound: expected 1 %s, got 2"},
		{"hintNumbersUpTo", "%d%d", "hintNumbersUpTo: expected 1 %d, got 2"},
		{"errAmbiguousNumber", "%d%d", "errAmbiguousNumber: expected 1 %d, got 2"},
		{"warnAmbiguousNumber", "%d%d", "warnAmbiguousNumber: expected 1 %d, got 2"},
		{"warnAmbiguousNumber", "n=%d p=%s%s", "warnAmbiguousNumber: expected 1 %s, got 2"},
		{"warnAmbiguousNumberSkipped", "%d%d", "warnAmbiguousNumberSkipped: expected 1 %d, got 2"},
		{"warnAmbiguousNumberSkipped", "n=%d p=%s%s", "warnAmbiguousNumberSkipped: expected 1 %s, got 2"},
		{"updateCheckUpToDate", "%s%s", "updateCheckUpToDate: expected 1 %s, got 2"},
		{"updateCheckAvailable", "%s%s%s", "updateCheckAvailable: expected 2 %s, got 3"},
		{"updateCheckNoReleases", "%s", "updateCheckNoReleases: expected 0 %s, got 1"},
		{"updateCheckDev", "%s%s", "updateCheckDev: expected 1 %s, got 2"},
		{"errUpdateCheck", "%v%v", "errUpdateCheck: expected 1 %v, got 2"},
	}
	if len(tests) != 38 {
		t.Fatalf("test table: expected 38 cases, got %d", len(tests))
	}
	for _, tc := range tests {
		t.Run(tc.key+"/"+tc.wantErr, func(t *testing.T) {
			dj := validDict(t)
			ptr, ok := stringFieldSetters(&dj)[tc.key]
			if !ok {
				t.Fatalf("no setter for json key %q", tc.key)
			}
			*ptr = tc.value

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
