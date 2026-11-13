package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `---
name: demo
description: A demo skill
---

# Demo

Do the demo thing.
`

func writeSkill(t *testing.T, dir, dirName, content string) {
	t.Helper()
	path := filepath.Join(dir, dirName)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingDir(t *testing.T) {
	skills, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("want nil error for missing dir, got %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("want no skills, got %v", skills)
	}
}

func TestLoadParsesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "demo", sample)
	writeSkill(t, dir, "no-frontmatter", "just a body\n")
	writeSkill(t, dir, "empty", "")
	os.WriteFile(filepath.Join(dir, "loose.md"), []byte("x"), 0o644)

	skills, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills (empty SKILL.md skipped), got %d: %v", len(skills), skills)
	}
	if skills[0].Name != "demo" || skills[0].Description != "A demo skill" {
		t.Errorf("frontmatter not parsed: %+v", skills[0])
	}
	if skills[1].Name != "no-frontmatter" {
		t.Errorf("want dir name fallback, got %q", skills[1].Name)
	}
}

func TestLoadSkillToolRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "demo", sample)
	skills, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	tool := LoadSkillTool{Skills: skills}

	out, err := tool.Execute(context.Background(), `{"name":"demo"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Do the demo thing.") {
		t.Errorf("body missing from output: %q", out)
	}
	if !strings.Contains(out, "Skill directory: "+filepath.Join(dir, "demo")) {
		t.Errorf("skill directory header missing: %q", out)
	}
	if strings.Contains(out, "description:") {
		t.Errorf("frontmatter should be stripped: %q", out)
	}

	if _, err := tool.Execute(context.Background(), `{"name":"nope"}`); err == nil {
		t.Error("want error for unknown skill")
	} else if !strings.Contains(err.Error(), "demo") {
		t.Errorf("unknown-skill error should list available skills: %v", err)
	}
	if _, err := tool.Execute(context.Background(), `{}`); err == nil {
		t.Error("want error for missing name")
	}
}

func TestCatalog(t *testing.T) {
	if Catalog(nil) != "" {
		t.Error("empty skill list should render empty catalog")
	}
	cat := Catalog([]Skill{{Name: "demo", Description: "A demo skill"}})
	for _, want := range []string{"demo", "A demo skill", "load_skill"} {
		if !strings.Contains(cat, want) {
			t.Errorf("catalog missing %q:\n%s", want, cat)
		}
	}
}
