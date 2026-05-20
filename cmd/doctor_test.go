package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentMdCopyInSync guards against drift between the canonical
// repo-root AGENT.md (what users read on GitHub) and the cmd/AGENT.md
// copy required by go:embed. If this fails, run:
//
//	cp AGENT.md cmd/AGENT.md
//
// from the repo root.
func TestAgentMdCopyInSync(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Tests run from the package directory (cmd/). Canonical lives one up.
	root := filepath.Join(cwd, "..", "AGENT.md")
	copy := filepath.Join(cwd, "AGENT.md")

	rootBytes, err := os.ReadFile(root)
	if err != nil {
		t.Skipf("repo-root AGENT.md not present (likely running outside the repo): %v", err)
		return
	}
	copyBytes, err := os.ReadFile(copy)
	if err != nil {
		t.Fatalf("cmd/AGENT.md missing — needed for go:embed: %v", err)
	}
	if string(rootBytes) != string(copyBytes) {
		t.Fatalf("AGENT.md and cmd/AGENT.md differ. Run: cp AGENT.md cmd/AGENT.md")
	}
}

func TestExtractMdSection_HappyPath(t *testing.T) {
	doc := `# Title

## 1. Quick Rules

**[Rule 1]** Do the thing.

**[Rule 2]** Don't do the other thing.

---

## 2. Next Section

Stuff.
`
	got := extractMdSection(doc, "## 1. Quick Rules")
	if !strings.Contains(got, "Rule 1") || !strings.Contains(got, "Rule 2") {
		t.Fatalf("missing rule body, got:\n%s", got)
	}
	if strings.Contains(got, "Next Section") || strings.Contains(got, "Stuff.") {
		t.Fatalf("section bled past next heading, got:\n%s", got)
	}
}

func TestExtractMdSection_StopsAtTripleDash(t *testing.T) {
	doc := `## 4. Pitfalls

Avoid X.

---

## 5. Reference

Tables here.
`
	got := extractMdSection(doc, "## 4. Pitfalls")
	if !strings.Contains(got, "Avoid X.") {
		t.Fatalf("body missing, got:\n%s", got)
	}
	if strings.Contains(got, "Tables here") {
		t.Fatalf("section bled past --- separator, got:\n%s", got)
	}
}

func TestExtractMdSection_MissingHeadingReturnsEmpty(t *testing.T) {
	doc := "## A\nstuff\n## B\nmore\n"
	got := extractMdSection(doc, "## Z. Nonexistent")
	if got != "" {
		t.Fatalf("expected empty result for missing heading, got %q", got)
	}
}

func TestExtractAgentRules_ContainsExpectedSections(t *testing.T) {
	// The embedded AGENT.md must contain both subsections. If either is
	// missing the user gets a half-empty rules file appended to their
	// CLAUDE.md, so this is worth guarding.
	got := extractAgentRules()
	if !strings.Contains(got, "Quick Rules") {
		t.Fatal("output missing Quick Rules section")
	}
	if !strings.Contains(got, "Pitfalls") {
		t.Fatal("output missing Pitfalls section")
	}
	if !strings.Contains(got, "AGENT.md") {
		t.Fatal("output missing pointer to full AGENT.md")
	}
	// Sanity: must include at least one [Rule N] anchor and one pitfall.
	if !strings.Contains(got, "[Rule 1]") {
		t.Fatal("output missing [Rule 1] anchor")
	}
}

func TestExtractAgentRules_NoTrailingBoilerplate(t *testing.T) {
	// The output must not include the next section ("## 5. Reference" or
	// later) — those are deliberately omitted to keep CLAUDE.md appends lean.
	got := extractAgentRules()
	if strings.Contains(got, "## 5. Reference") {
		t.Fatal("output bled into Reference section")
	}
	if strings.Contains(got, "## 6. When this doc is wrong") {
		t.Fatal("output bled into footer section")
	}
}
