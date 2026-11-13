// Package skills discovers SKILL.md instruction files on disk and exposes
// them to the agent as a catalog (name + description) plus a tool that loads
// the full body on demand.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileName is the file every skill directory must contain.
const FileName = "SKILL.md"

// Skill is one discovered skill instruction file.
type Skill struct {
	Name        string
	Description string
	Path        string
}

// Load scans dir for <name>/SKILL.md directories. A missing dir is not an
// error; it yields no skills. Frontmatter name wins over the directory name.
func Load(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name(), FileName)
		data, err := os.ReadFile(path)
		if err != nil || len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		fields, _ := parseFrontmatter(string(data))
		skill := Skill{
			Name:        fields["name"],
			Description: fields["description"],
			Path:        path,
		}
		if skill.Name == "" {
			skill.Name = entry.Name()
		}
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// Catalog renders the skill index appended to the system prompt: one line per
// skill plus the rule telling the model to load a skill when a task matches.
func Catalog(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Skills (task-specific instruction files):\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
	}
	b.WriteString("When the user's task matches a skill, call the load_skill tool with its name and follow the returned instructions.")
	return b.String()
}

// parseFrontmatter splits a leading "---" delimited block into key/value
// fields and returns the remaining body. Unterminated frontmatter is treated
// as plain body.
func parseFrontmatter(data string) (fields map[string]string, body string) {
	lines := strings.Split(data, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return map[string]string{}, data
	}
	fields = map[string]string{}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			return fields, strings.Join(lines[i+1:], "\n")
		}
		if key, value, ok := strings.Cut(line, ":"); ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return map[string]string{}, data
}
