package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// cmdList implements `list [active|open|wip|done|closed|archive|all] [-P project]`
// with the default filter active. The archive filter lists tickets under
// tickets/archive/ instead of the main directory. An empty first argument
// falls back to the default (bash `want="${1:-active}"` treats an empty
// $1 as unset). T-0040: -P flag filters by exact project name match
// (case-sensitive); unknown flags error (deliberate deviation from bash
// parity for typo protection). Signature per wave-1 dispatch contract
// (wiki 8bd93a4e, A1).
func cmdList(st *store.Store, args []string, who, project string, lang domain.Lang, stdout, stderr io.Writer) int {
	// Parse flags: -P can appear anywhere, --json is bare flag, positional filter = first non-flag arg
	var projectFilter string
	var jsonMode bool
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-P" {
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "ticket: флаг -P требует значение")
				return 1
			}
			i++
			projectFilter = args[i]
			if strings.TrimSpace(projectFilter) == "" {
				fmt.Fprintln(stderr, "ticket: -P требует непустое имя проекта")
				return 1
			}
		} else if args[i] == "--json" {
			if jsonMode {
				fmt.Fprintln(stderr, "ticket: повторный флаг --json")
				return 1
			}
			jsonMode = true
		} else if strings.HasPrefix(args[i], "-") {
			// Unknown flag (T-0040: deliberate deviation from bash parity)
			fmt.Fprintf(stderr, "ticket: неизвестный флаг: %s\n", args[i])
			return 1
		} else {
			positional = append(positional, args[i])
		}
	}
	want := "active"
	if len(positional) >= 1 && positional[0] != "" {
		want = positional[0]
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, "ticket: список принимает не более одного аргумента (фильтр)")
		return 1
	}
	if !isListFilter(want) {
		fmt.Fprintln(stderr, "ticket: фильтр — один из: active open wip done closed archive all")
		return 1
	}

	var tickets []domain.Ticket
	var warnings []store.ParseWarning

	if want == "archive" {
		tickets, warnings = st.ListArchive()
	} else {
		tickets, warnings = st.List()
	}

	// Parse warnings are non-fatal: list shows successfully parsed tickets and returns 0. Broken ticket files are logged to stderr but do not block shell pipelines or automation.
	for _, w := range warnings {
		fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
	}
	// Filter and collect output tickets
	var output []domain.Ticket
	uniqueProjects := make(map[string]bool)
	for i := range tickets {
		t := tickets[i]
		if want != "archive" && !matchFilter(t.Status, want) {
			continue
		}
		// Apply -P filter (exact match, case-sensitive)
		if projectFilter != "" && t.Project != projectFilter {
			continue
		}
		output = append(output, t)
		uniqueProjects[t.Project] = true
	}
	if jsonMode {
		// JSON output: DTO with exactly 6 fields, RFC3339 created, default HTML escaping
		type ticketDTO struct {
			Number   int    `json:"number"`
			Status   string `json:"status"`
			Type     string `json:"type"`
			Priority string `json:"priority"`
			Title    string `json:"title"`
			Created  string `json:"created"`
		}
		dtos := make([]ticketDTO, len(output))
		for i, t := range output {
			dtos[i] = ticketDTO{
				Number:   t.Number,
				Status:   string(t.Status),
				Type:     string(t.Type),
				Priority: string(t.Priority),
				Title:    t.Title,
				Created:  t.Created.Format("2006-01-02T15:04:05Z07:00"),
			}
		}
		enc := json.NewEncoder(stdout)
		if err := enc.Encode(dtos); err != nil {
			fmt.Fprintf(stderr, "ticket: JSON encode error: %v\n", err)
			return 1
		}
		return 0
	}

	if len(output) == 0 {
		// Determine filter description for noTickets message
		filterDesc := want
		if projectFilter != "" {
			filterDesc = want + " -P " + projectFilter
		}
		fmt.Fprintln(stdout, domain.NoTickets(lang, filterDesc))
		return 0
	}
	// Print with project column when len(uniqueProjects) > 1
	showProjectCol := len(uniqueProjects) > 1
	for _, t := range output {
		if showProjectCol {
			fmt.Fprintf(stdout, "T-%04d  %-10s  %-7s  %s\n", t.Number, t.Project, t.Status, string(t.Type)+": "+t.Title)
		} else {
			fmt.Fprintf(stdout, "T-%04d  %-7s  %s\n", t.Number, t.Status, string(t.Type)+": "+t.Title)
		}
	}
	return 0
}

// isListFilter reports whether want is a valid list filter.
func isListFilter(want string) bool {
	switch want {
	case "active", "open", "wip", "done", "closed", "archive", "all":
		return true
	}
	return false
}

// matchFilter applies the list filter to a ticket status.
func matchFilter(st domain.Status, want string) bool {
	switch want {
	case "all":
		return true
	case "active":
		return st == domain.StatusOpen || st == domain.StatusWip
	case "open":
		return st == domain.StatusOpen
	case "wip":
		return st == domain.StatusWip
	case "done":
		return st == domain.StatusDone
	case "closed":
		return st == domain.StatusClosed
	}
	return false
}
