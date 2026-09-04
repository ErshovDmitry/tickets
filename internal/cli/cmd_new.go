package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"ticket/internal/domain"
	"ticket/internal/store"
)

// newFlags holds the parsed options of the `new` command.
type newFlags struct {
	title           string
	typ             string
	prio            string
	details         string
	who             string
	projectOverride string // -P value (T-0040)
}

// cmdNew implements `new "<кратко>" [-t T] [-p P] [-d D] [-w W] [-P PROJECT]`.
// The title must come before any flag (bash: title=$1; shift).
// lang selects the usage text language for argument errors AND the file
// format language of the created ticket (T-0036: both come from the same
// resolved lang, plan §C). Signature per wave-1 dispatch contract (wiki
// 8bd93a4e, A1).
func cmdNew(st *store.Store, args []string, who, project string, lang domain.Lang, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stdout, lang)
		return 1
	}
	// Deliberate deviation from bash parity (T-0041): reject titles
	// starting with '-' to prevent accidental flag-as-title bugs.
	if strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(stderr, "ticket: краткое описание не может начинаться с '-'")
		return 1
	}
	f, ok := parseNewFlags(args[0], args[1:], who, stderr)
	if !ok {
		return 1
	}
	// -P override (T-0040)
	if f.projectOverride != "" {
		project = f.projectOverride
	}
	// Empty/whitespace-only titles are rejected before Create, matching
	// domain.Validate(Strict) ("краткое описание не может быть пустым").
	if strings.TrimSpace(f.title) == "" {
		fmt.Fprintln(stderr, "ticket: краткое описание не может быть пустым")
		return 1
	}
	typ, okType := typeByName(f.typ)
	prio, okPrio := priorityByName(f.prio)
	if !okType {
		fmt.Fprintln(stderr, "ticket: тип — один из: BUG OPS TD ENH")
		return 1
	}
	if !okPrio {
		fmt.Fprintln(stderr, "ticket: приоритет — один из: low normal high")
		return 1
	}
	// firstUse check BEFORE Create (T-0040): known projects = unique
	// t.Project from List+ListArchive
	firstUse := false
	if project != "" {
		known := make(map[string]bool)
		allTickets, warnings := st.List()
		for _, w := range warnings {
			fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
		}
		for _, t := range allTickets {
			known[t.Project] = true
		}
		archiveTickets, warnings2 := st.ListArchive()
		for _, w := range warnings2 {
			fmt.Fprintf(stderr, "ticket: %s\n", w.Error())
		}
		for _, t := range archiveTickets {
			known[t.Project] = true
		}
		if !known[project] {
			firstUse = true
		}
	}
	n, err := st.Create(newTicket(f, typ, prio, project, lang))
	if err != nil {
		return createError(stderr, st, err)
	}
	fmt.Fprintln(stdout, filepath.Join(st.Dir, domain.Filename(n, domain.StatusOpen)))
	// Print warning to stderr if project is firstUse, exit 0 (T-0040)
	if firstUse {
		fmt.Fprintln(stderr, domain.WarnNewProject(lang, project))
	}
	return 0
}

// parseNewFlags consumes `-t/-p/-d/-w/-P` value pairs exactly like bash:
// blind value consumption, unknown flag dies at once, values are
// validated after the loop. ok=false after printing the error.
func parseNewFlags(title string, flags []string, who string, stderr io.Writer) (newFlags, bool) {
	f := newFlags{title: title, typ: "BUG", prio: "normal", who: who}
	for i := 0; i < len(flags); i++ {
		arg := flags[i]
		if !isNewFlag(arg) {
			fmt.Fprintf(stderr, "ticket: неизвестный аргумент: %s\n", arg)
			return f, false
		}
		if i+1 >= len(flags) {
			fmt.Fprintf(stderr, "ticket: флаг %s требует значение\n", arg)
			return f, false
		}
		i++
		switch arg {
		case "-t":
			f.typ = flags[i]
		case "-p":
			f.prio = flags[i]
		case "-d":
			f.details = flags[i]
		case "-w":
			f.who = flags[i]
		case "-P":
			if strings.TrimSpace(flags[i]) == "" {
				fmt.Fprintln(stderr, "ticket: -P требует непустое имя проекта")
				return f, false
			}
			f.projectOverride = flags[i]
		}
	}
	return f, true
}

// isNewFlag reports whether arg is one of the value-taking flags of new.
func isNewFlag(arg string) bool {
	switch arg {
	case "-t", "-p", "-d", "-w", "-P":
		return true
	}
	return false
}

// typeByName maps a -t value to its enum constant.
func typeByName(s string) (domain.Type, bool) {
	switch s {
	case "BUG":
		return domain.TypeBUG, true
	case "OPS":
		return domain.TypeOPS, true
	case "TD":
		return domain.TypeTD, true
	case "ENH":
		return domain.TypeENH, true
	}
	return domain.TypeBUG, false
}

// priorityByName maps a -p value to its enum constant.
func priorityByName(s string) (domain.Priority, bool) {
	switch s {
	case "low":
		return domain.PriorityLow, true
	case "normal":
		return domain.PriorityNormal, true
	case "high":
		return domain.PriorityHigh, true
	}
	return domain.PriorityLow, false
}

// newTicket assembles the in-memory ticket for Create. lang becomes
// tk.Lang: RenderNewTicket renders headers, meta and stubs from that
// language's dictionary (T-0036). The creation journal entry
// (From==To==open, empty comment) matches §2 semantics.
func newTicket(f newFlags, typ domain.Type, prio domain.Priority, project string, lang domain.Lang) *domain.Ticket {
	now := time.Now()
	return &domain.Ticket{
		Status:   domain.StatusOpen,
		Type:     typ,
		Priority: prio,
		Title:    f.title,
		Details:  f.details,
		Who:      f.who,
		Project:  project,
		Lang:     lang,
		Created:  now,
		Journal: []domain.JournalEntry{{
			At:   now,
			From: domain.StatusOpen,
			To:   domain.StatusOpen,
			Who:  f.who,
		}},
	}
}

// createError reports a failed Create on stderr and yields exit code 1.
func createError(stderr io.Writer, st *store.Store, err error) int {
	if errors.Is(err, store.ErrCollision) {
		fmt.Fprintf(stderr, "ticket: файл %s уже существует\n", collisionPath(st))
		return 1
	}
	fmt.Fprintf(stderr, "ticket: %v\n", err)
	return 1
}

// collisionPath best-effort reconstructs the path of the file that
// blocked Create: the store does not expose the loser of the O_EXCL
// race, so the current maximum+1 candidate is reported (§6 shape).
func collisionPath(st *store.Store) string {
	tickets, _ := st.List()
	max := 0
	for i := range tickets {
		if tickets[i].Number > max {
			max = tickets[i].Number
		}
	}
	return filepath.Join(st.Dir, domain.Filename(max+1, domain.StatusOpen))
}
