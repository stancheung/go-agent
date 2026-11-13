package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"go-agent/config"
	"go-agent/extensions/skills"
	"go-agent/internal/agent"
	"go-agent/internal/ai"
)

type Repl struct {
	Cfg *config.Config
}

func NewRepl() *Repl {
	cfg := config.NewConfig()
	return &Repl{cfg}
}

func (repl *Repl) Run(ctx context.Context) {
	provider := ai.NewProvider(ctx, repl.Cfg.BaseURL, repl.Cfg.Model, repl.Cfg.APIKey)
	ag := agent.New(ctx, provider)

	skillList, err := skills.Load("skills")
	if err != nil {
		fmt.Printf("Warning: could not load skills: %v\n", err)
	}
	if len(skillList) > 0 {
		ag.RegisterTool(skills.LoadSkillTool{Skills: skillList})
		ag.SystemPrompt += "\n" + skills.Catalog(skillList)
	}

	// reader := bufio.NewReader(os.Stdin)
	var input string
	for {
		if ctx.Err() != nil {
			return
		}

		scanner := bufio.NewScanner(os.Stdin)
		fmt.Println("You: ")

		// Read the next line of input
		if scanner.Scan() {
			input = scanner.Text()
		}

		// Check for any reading errors
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading standard input:", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		fmt.Println()

		resp, err := ag.Run(ctx, input)
		if err != nil {
			fmt.Printf("Error getting a response from LLM: %v\n\n", err)
			continue
		}
		fmt.Println("Agent: ")
		fmt.Println(resp)
		fmt.Println()
	}
}
