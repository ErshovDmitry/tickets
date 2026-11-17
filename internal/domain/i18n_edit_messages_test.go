package domain

// T-0187: characterization tests for the four `ticket edit` dictionary
// keys (errEditUnknownFlag, errEditFieldNewline, errEditAlreadyField,
// errProjectEmpty): placeholder-count errors through the shared validation
// pipeline, helper rendering in RU/EN, EN fallback for unknown languages,
// and ru/en JSON key parity. The empty-value path for the new keys is
// covered automatically by TestValidateDictEmptyFields via the setter map.

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestValidateDictEditKeys breaks exactly one placeholder count at a time
// for the two keyed edit messages; error text is pinned byte-for-byte.
func TestValidateDictEditKeys(t *testing.T) {
	tests := []struct {
		key     string
		value   string
		wantErr string
	}{
		{"errEditUnknownFlag", "%s%s", "errEditUnknownFlag: expected 1 %s, got 2"},
		{"errEditAlreadyField", "n=%d%d f=%s", "errEditAlreadyField: expected 1 %d, got 2"},
		{"errEditAlreadyField", "n=%d f=%s%s", "errEditAlreadyField: expected 1 %s, got 2"},
		{"errEditAlreadyField", "n=%d f=%s v=%q%q", "errEditAlreadyField: expected 1 %q, got 2"},
	}
	for _, tc := range tests {
		t.Run(tc.key+"/"+tc.wantErr, func(t *testing.T) {
			dj := validDict(t)
			*stringFieldSetters(&dj)[tc.key] = tc.value

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

// TestEditMessagesRender pins the RU/EN rendering of every new helper and
// the EN fallback for an unknown language.
func TestEditMessagesRender(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ru unknown flag", ErrEditUnknownFlag(LangRU, "--bogus"), "ticket: неизвестный флаг для команды edit: --bogus"},
		{"en unknown flag", ErrEditUnknownFlag(LangEN, "--bogus"), "ticket: unknown flag for edit command: --bogus"},
		{"ru newline", ErrEditFieldNewline(LangRU), "ticket: новое значение не может содержать перевод строки"},
		{"en newline", ErrEditFieldNewline(LangEN), "ticket: edit value must not contain a line break"},
		{"ru already", ErrEditAlreadyField(LangRU, 7, "title", "Старый"), "ticket: у тикета 7 поле title уже \"Старый\""},
		{"en already", ErrEditAlreadyField(LangEN, 7, "title", "Old"), "ticket: ticket 7 already has title \"Old\""},
		{"ru project empty", ErrProjectEmpty(LangRU), "ticket: проект не может быть пустым"},
		{"en project empty", ErrProjectEmpty(LangEN), "ticket: project must not be empty"},
		{"fallback unknown lang", ErrProjectEmpty(Lang("zz")), "ticket: project must not be empty"},
		{"fallback unknown lang flag", ErrEditUnknownFlag(Lang("zz"), "-t"), "ticket: unknown flag for edit command: -t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// TestEditDictsKeyParity requires the ru/en JSON documents to carry exactly
// the same key set, so a missing translation fails here, not in production.
func TestEditDictsKeyParity(t *testing.T) {
	ru := editKeys(t, "langs/ru.json")
	en := editKeys(t, "langs/en.json")
	for k := range ru {
		if _, ok := en[k]; !ok {
			t.Errorf("en.json missing key %q (present in ru.json)", k)
		}
	}
	for k := range en {
		if _, ok := ru[k]; !ok {
			t.Errorf("ru.json missing key %q (present in en.json)", k)
		}
	}
}

// editKeys reads the four edit keys from one embedded dictionary document.
func editKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	raw, err := langsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var dj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &dj); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	out := map[string]bool{}
	for _, k := range []string{"errEditUnknownFlag", "errEditFieldNewline", "errEditAlreadyField", "errProjectEmpty"} {
		v, ok := dj[k]
		if !ok {
			t.Fatalf("%s: key %q missing", path, k)
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil || s == "" {
			t.Fatalf("%s: key %q must be a non-empty string", path, k)
		}
		out[k] = true
	}
	return out
}

// TestLoadDictEditFields pins that loadDict (the shared loader, with the
// full validateDictJSON pipeline) picks the four edit messages up from the
// embedded documents and lands them on the shared dict.
func TestLoadDictEditFields(t *testing.T) {
	for _, lang := range []Lang{LangRU, LangEN} {
		raw, err := langsFS.ReadFile("langs/" + string(lang) + ".json")
		if err != nil {
			t.Fatalf("read langs/%s.json: %v", lang, err)
		}
		d, err := loadDict(raw)
		if err != nil {
			t.Fatalf("loadDict(langs/%s.json): %v", lang, err)
		}
		if strings.Contains(d.errEditAlreadyField, "%!") || !strings.Contains(d.errEditAlreadyField, "%d") {
			t.Errorf("%s errEditAlreadyField = %q, want a %%d slot", lang, d.errEditAlreadyField)
		}
		if d.errEditUnknownFlag == "" || d.errEditFieldNewline == "" || d.errProjectEmpty == "" {
			t.Errorf("%s dict has empty edit fields: %+v", lang, d)
		}
	}
}

// TestGetDictEditFields guards the runtime seam: getDict serves the edit
// messages for RU/EN and falls back to the EN dict for unknown languages.
func TestGetDictEditFields(t *testing.T) {
	if getDict(LangRU).errEditAlreadyField == "" {
		t.Error("getDict(RU).errEditAlreadyField is empty")
	}
	en := getDict(LangEN)
	if en.errEditUnknownFlag == "" || en.errEditFieldNewline == "" ||
		en.errEditAlreadyField == "" || en.errProjectEmpty == "" {
		t.Errorf("getDict(EN) has empty edit fields: %+v", en)
	}
	if zz := getDict(Lang("zz")); zz != en {
		t.Errorf("getDict(zz) = %+v, want the EN dict %+v", zz, en)
	}
}
