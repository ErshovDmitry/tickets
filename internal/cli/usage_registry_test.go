package cli

import (
	"bytes"
	"strings"
	"testing"

	"ticket/internal/domain"
)

func TestUsageRU(t *testing.T) {
	var b bytes.Buffer
	usage(&b, domain.LangRU)
	output := b.String()
	if !strings.Contains(output, "ticket — тикеты проекта") {
		t.Error("missing RU usage marker")
	}
}

func TestUsageEN(t *testing.T) {
	var b bytes.Buffer
	usage(&b, domain.LangEN)
	output := b.String()
	if !strings.Contains(output, "ticket — project tickets") {
		t.Error("missing EN usage marker")
	}
}
