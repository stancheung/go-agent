package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/extensions/tools"
)

// LoadSkillTool returns the body of a SKILL.md so the model can follow it.
// The skill list is snapshotted at startup; new files need a restart.
type LoadSkillTool struct {
	Skills []Skill
}

func (LoadSkillTool) Name() string { return "load_skill" }
func (LoadSkillTool) Description() string {
	return "Load the full instructions of a skill by name. Use when the task matches one of the skills listed in the system prompt."
}
func (LoadSkillTool) Parameters() any {
	return tools.Schema([]string{"name"}, map[string]any{
		"name": tools.StrParam("Name of the skill to load"),
	})
}

func (t LoadSkillTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if args.Name == "" {
		return "", fmt.Errorf("missing required argument: name")
	}

	names := make([]string, 0, len(t.Skills))
	for _, s := range t.Skills {
		names = append(names, s.Name)
		if s.Name != args.Name {
			continue
		}
		data, err := os.ReadFile(s.Path)
		if err != nil {
			return "", err
		}
		_, body := parseFrontmatter(string(data))
		dir := filepath.Dir(s.Path)
		example := filepath.Join(dir, "scripts", "example.py")
		return fmt.Sprintf("Skill directory: %s\nIMPORTANT: your shell's working directory is NOT the skill directory. Relative paths like scripts/ or references/ in these instructions do NOT exist there — always prefix them with the skill directory, e.g. python \"%s\" <args>.\n\n%s",
			dir, example, strings.TrimSpace(body)), nil
	}
	return "", fmt.Errorf("no skill named %q; available: %s", args.Name, strings.Join(names, ", "))
}
