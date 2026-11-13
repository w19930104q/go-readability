package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	readability "github.com/go-shiori/go-readability"
)

const cliRevisionPage = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>CLI Revision Test</title></head>
<body>
<article>
<p>The committee spent the morning reviewing every paragraph of the draft agreement before taking a short break for lunch together near the old library building downtown.</p>
<p>The committee spent the morning reviewing every paragraph of the draft agreement before taking a short break for lunch together near the old library building downtown.</p>
<p>Final paragraph says that <del>the old rule applies here and stays in force for another decade at least</del><ins>` + "the new rule replaces the previous one starting next spring for every member state without exceptions whatsoever" + `</ins> in every case.</p>
</article>
</body>
</html>`

func writeCLITestPage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(cliRevisionPage), 0o600); err != nil {
		t.Fatalf("write test page: %v", err)
	}
	return path
}

func TestCLIContentRevisionViews(t *testing.T) {
	path := writeCLITestPage(t)

	defaultContent, err := getContent(path, false, false, readability.RevisionViewNone)
	if err != nil {
		t.Fatalf("default view: %v", err)
	}
	if !strings.Contains(defaultContent, "<ins") || !strings.Contains(defaultContent, "<del") {
		t.Errorf("default view should keep revision marks, got: %s", defaultContent)
	}

	currentContent, err := getContent(path, false, false, readability.RevisionViewCurrent)
	if err != nil {
		t.Fatalf("current view: %v", err)
	}
	if strings.Contains(currentContent, "<ins") || strings.Contains(currentContent, "<del") {
		t.Errorf("current view should contain no revision marks, got: %s", currentContent)
	}
	if !strings.Contains(currentContent, "the new rule replaces") {
		t.Errorf("current view should keep inserted text")
	}
	if strings.Contains(currentContent, "the old rule applies") {
		t.Errorf("current view should drop deleted text")
	}

	originalContent, err := getContent(path, false, false, readability.RevisionViewOriginal)
	if err != nil {
		t.Fatalf("original view: %v", err)
	}
	if strings.Contains(originalContent, "<ins") || strings.Contains(originalContent, "<del") {
		t.Errorf("original view should contain no revision marks, got: %s", originalContent)
	}
	if !strings.Contains(originalContent, "the old rule applies") {
		t.Errorf("original view should keep deleted text")
	}
	if strings.Contains(originalContent, "the new rule replaces") {
		t.Errorf("original view should drop inserted text")
	}

	// The text-only output must follow the selected view as well.
	currentText, err := getContent(path, false, true, readability.RevisionViewCurrent)
	if err != nil {
		t.Fatalf("current text view: %v", err)
	}
	if strings.Contains(currentText, "the old rule applies") || !strings.Contains(currentText, "the new rule replaces") {
		t.Errorf("text-only output does not follow the current view: %q", currentText)
	}
}

func TestCLIParseRevisionViewFlag(t *testing.T) {
	valid := map[string]readability.RevisionView{
		"":         readability.RevisionViewNone,
		"none":     readability.RevisionViewNone,
		"current":  readability.RevisionViewCurrent,
		"original": readability.RevisionViewOriginal,
	}
	for input, want := range valid {
		got, err := readability.ParseRevisionView(input)
		if err != nil || got != want {
			t.Errorf("ParseRevisionView(%q) = %v, %v; want %v, nil", input, got, err, want)
		}
	}

	if _, err := readability.ParseRevisionView("bogus"); err == nil {
		t.Error("expected error for invalid revision view value")
	}
}

func TestCLISourceFileUnreadable(t *testing.T) {
	// A non-existent file must surface an error rather than panic.
	if _, err := getContent(filepath.Join(t.TempDir(), "missing.html"), false, false, readability.RevisionViewCurrent); err == nil {
		t.Error("expected an error for a missing source file")
	}
}
