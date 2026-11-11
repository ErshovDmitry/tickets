// This file holds the environment resolvers shared by every command:
// whoFrom (the journal author) and the language resolvers langFrom and
// langPrefix. It is a pure code-motion split of cli.go, made for T-0125 so
// the dispatch wiring for the new normalize command fits the 300-line file
// budget: no behaviour, signature or API change.
package cli

import (
	"strings"

	"ticket/internal/domain"
)

// whoFrom resolves the current user: TICKET_WHO, then USER, then USERNAME,
// then "agent"; USERNAME covers Windows hosts where USER is normally unset.
func whoFrom(env map[string]string) string {
	if who := env["TICKET_WHO"]; who != "" {
		return who
	}
	if who := env["USER"]; who != "" {
		return who
	}
	if who := env["USERNAME"]; who != "" {
		return who
	}
	return "agent"
}

// langFrom resolves the ticket language (T-0036: one language for both
// the new-ticket file format and the usage text): TICKET_LANG, then
// LC_ALL, then LANG. An empty value counts as unset and falls through to
// the next variable; the first non-empty one decides via langPrefix. All
// unset defaults to LangRU (language selection itself is new in T-0026
// and has no pre-Go predecessor) — but only while RU is actually
// registered. A degraded initialisation (ru.json broken) must not hand
// out LangRU: domain.getDict would silently substitute EN while callers
// believe RU was selected. EN is mandatory, so it is the safe default.
func langFrom(env map[string]string) domain.Lang {
	for _, key := range []string{"TICKET_LANG", "LC_ALL", "LANG"} {
		if v := env[key]; v != "" {
			return langPrefix(v)
		}
	}
	if isKnownLangFn(domain.LangRU) {
		return domain.LangRU
	}
	return domain.LangEN
}

// langPrefix maps a locale value to a domain.Lang: the prefix up to the
// first '_' or '.' decides, case-insensitive ("en_US.UTF-8" → en,
// "ru_RU" → ru). If the prefix matches a known language in the registry,
// that language is selected; otherwise falls back to LangEN (T-0036
// deliberate change: unknown locales used to get the Russian text).
func langPrefix(v string) domain.Lang {
	if i := strings.IndexAny(v, "_."); i >= 0 {
		v = v[:i]
	}
	lang := domain.Lang(strings.ToLower(v))
	if domain.IsKnownLang(lang) {
		return lang
	}
	return domain.LangEN // fallback for unknown locales
}
