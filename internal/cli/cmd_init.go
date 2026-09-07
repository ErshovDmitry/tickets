package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func cmdInit(stdout, stderr io.Writer) int {
	cwd, err := os.Getwd()
	if err != nil {
		return initError(stderr, err)
	}
	return initProject(cwd, stdout, stderr)
}
func initProject(cwd string, stdout, stderr io.Writer) int {
	tickets := filepath.Join(cwd, "tickets")
	if info, err := os.Stat(tickets); err == nil && !info.IsDir() {
		return conflict(stderr, tickets)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return initError(stderr, err)
	}
	if err := os.MkdirAll(filepath.Join(tickets, "archive"), 0755); err != nil {
		return initError(stderr, err)
	}
	fmt.Fprintf(stdout, "Инициализировано: %s\n", tickets)
	return 0
}
func initError(w io.Writer, e error) int {
	fmt.Fprintf(w, "ticket: Ошибка инициализации: %v\n", e)
	return 1
}
func conflict(w io.Writer, p string) int {
	fmt.Fprintf(w, "ticket: Конфликт: %s\n", p)
	return 1
}
