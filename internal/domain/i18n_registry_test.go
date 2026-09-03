package domain

import (
	"strings"
	"testing"
)

func TestRegistryLoadsAllFiles(t *testing.T) {
	if !IsKnownLang(LangRU) {
		t.Error("RU not in registry")
	}
	if !IsKnownLang(LangEN) {
		t.Error("EN not in registry")
	}
}

func TestAllDictsOrder(t *testing.T) {
	// LOCK D4 v2: RU first
	if len(allDicts) < 2 {
		t.Fatalf("allDicts too short: got %d, want >=2", len(allDicts))
	}
	if allDicts[0].lang != LangRU {
		t.Fatalf("allDicts order broken: RU must be first, got %s", allDicts[0].lang)
	}
	if allDicts[1].lang != LangEN {
		t.Fatalf("allDicts[1] must be EN, got %s", allDicts[1].lang)
	}
}

func TestGetDictFallback(t *testing.T) {
	unknownLang := Lang("zz")
	if d := getDict(unknownLang); d != getDict(LangEN) {
		t.Error("getDict fallback должен быть EN")
	}
}

func TestIsKnownLang(t *testing.T) {
	if !IsKnownLang(LangRU) {
		t.Error("IsKnownLang(LangRU) should be true")
	}
	if !IsKnownLang(LangEN) {
		t.Error("IsKnownLang(LangEN) should be true")
	}
	if IsKnownLang(Lang("unknown")) {
		t.Error("IsKnownLang(unknown) should be false")
	}
}

func TestJSONSchemaValid(t *testing.T) {
	for _, name := range []string{"langs/ru.json", "langs/en.json"} {
		data, err := langsFS.ReadFile(name)
		if err != nil {
			t.Errorf("%s read failed: %v", name, err)
			continue
		}
		if _, err := loadDict(data); err != nil {
			t.Errorf("%s parse failed: %v", name, err)
		}
	}
}

func TestRegistryCoversEmbeddedFiles(t *testing.T) {
	// Verify that registry contains exactly the embedded langs/*.json files
	entries, err := langsFS.ReadDir("langs")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	var embeddedCodes []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		code := strings.ToLower(strings.TrimSuffix(entry.Name(), ".json"))
		embeddedCodes = append(embeddedCodes, code)
	}

	if len(registry) != len(embeddedCodes) {
		t.Errorf("registry size mismatch: got %d, embedded %d", len(registry), len(embeddedCodes))
	}

	for _, code := range embeddedCodes {
		if _, ok := registry[Lang(code)]; !ok {
			t.Errorf("embedded %s.json not in registry", code)
		}
	}

	for code := range registry {
		found := false
		for _, ec := range embeddedCodes {
			if string(code) == ec {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("registry[%s] not matched by embedded file", code)
		}
	}
}
