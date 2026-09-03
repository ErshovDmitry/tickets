package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ticket/internal/cli"
)

// TestNewProjectOverride verifies that -P overrides the default project
// (T-0040).
func TestNewProjectOverride(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	args := []string{"new", "Test ticket", "-P", "custom-project"}
	if code := cli.Run(args, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new -P) = %d, want 0; stderr: %q", code, stderr.String())
	}
	created, err := os.ReadFile(filepath.Join(dir, "T-0001-open.md"))
	if err != nil {
		t.Fatalf("created ticket not readable: %v", err)
	}
	if !strings.Contains(string(created), "Project (Проект): custom-project") {
		t.Errorf("ticket missing overridden project 'custom-project':\n%s", created)
	}
}

// TestNewProjectFirstUse verifies the warning appears on first-use project
// (T-0040).
func TestNewProjectFirstUse(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	args := []string{"new", "First ticket", "-P", "newproject"}
	if code := cli.Run(args, env, &stdout, &stderr); code != 0 {
		t.Fatalf("Run(new -P newproject) = %d, want 0; stderr: %q", code, stderr.String())
	}
	stderrOut := stderr.String()
	if !strings.Contains(stderrOut, "newproject") || !strings.Contains(stderrOut, "впервые") {
		t.Errorf("stderr missing first-use warning for 'newproject': %q", stderrOut)
	}
	// Verify ticket file was created
	filePath := filepath.Join(dir, "T-0001-open.md")
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("ticket file not created: %v", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("cannot read created ticket: %v", err)
	}
	if !strings.Contains(string(data), "newproject") {
		t.Errorf("created ticket missing project 'newproject': %s", data)
	}
}

// TestNewProjectRepeatUse verifies no warning on repeat project (T-0040).
func TestNewProjectRepeatUse(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	// Create first ticket with proj1
	var stdout1, stderr1 bytes.Buffer
	if code := cli.Run([]string{"new", "First", "-P", "proj1"}, env, &stdout1, &stderr1); code != 0 {
		t.Fatalf("first new = %d, stderr: %q", code, stderr1.String())
	}
	if !strings.Contains(stderr1.String(), "впервые") {
		t.Errorf("first ticket stderr missing warning: %q", stderr1.String())
	}
	// Create second ticket with proj1 again
	var stdout2, stderr2 bytes.Buffer
	if code := cli.Run([]string{"new", "Second", "-P", "proj1"}, env, &stdout2, &stderr2); code != 0 {
		t.Fatalf("second new = %d, stderr: %q", code, stderr2.String())
	}
	if strings.Contains(stderr2.String(), "впервые") {
		t.Errorf("second ticket stderr has warning (should not): %q", stderr2.String())
	}
}

// TestNewProjectEmptyValue verifies empty -P value is rejected (T-0040).
func TestNewProjectEmptyValue(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	args := []string{"new", "Test", "-P", ""}
	if code := cli.Run(args, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(new -P empty) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "непустое") {
		t.Errorf("stderr missing non-empty error: %q", stderr.String())
	}
}

// TestNewProjectWhitespaceValue verifies whitespace-only -P value is rejected (T-0040).
func TestNewProjectWhitespaceValue(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	args := []string{"new", "Test", "-P", "   "}
	if code := cli.Run(args, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(new -P whitespace) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "непустое") {
		t.Errorf("stderr missing non-empty error: %q", stderr.String())
	}
}

// TestNewProjectMissingValue verifies -P without value is rejected (T-0040).
func TestNewProjectMissingValue(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"TICKETS_DIR": dir}
	var stdout, stderr bytes.Buffer
	args := []string{"new", "Test", "-P"}
	if code := cli.Run(args, env, &stdout, &stderr); code != 1 {
		t.Fatalf("Run(new -P without value) = %d, want 1; stderr: %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "требует значение") {
		t.Errorf("stderr missing 'requires value' error: %q", stderr.String())
	}
}
