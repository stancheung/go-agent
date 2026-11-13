package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type BashTool struct{}

func (BashTool) Name() string { return "bash" }
func (BashTool) Description() string {
	return fmt.Sprintf("Execute a shell command and return its combined stdout and stderr, prefixed with a leading 'exit: N' line. Shell: %s. Rules: %s",
		ShellDescription(), ShellGuidelines())
}
func (BashTool) Parameters() any {
	return Schema([]string{"command"}, map[string]any{
		"command": StrParam("The shell command to execute"),
		"workdir": StrParam("Working directory for the command; omit to inherit the agent's"),
		"timeout": IntParam("Timeout in seconds; omit for the default of 120"),
	})
}

func (BashTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
		Timeout int    `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	if strings.TrimSpace(args.Command) == "" {
		return "", fmt.Errorf("missing required argument: command")
	}
	if runtime.GOOS == "windows" {
		if msg := posixismError(args.Command); msg != "" {
			return "", errors.New(msg)
		}
	}

	timeout := args.Timeout
	if timeout <= 0 {
		timeout = defaultBashTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var argv []string
	if runtime.GOOS == "windows" {
		argv = []string{"-NoProfile", "-NonInteractive", "-Command", args.Command}
	} else {
		argv = []string{"-c", args.Command}
	}
	cmd := exec.CommandContext(cctx, shellProgram(), argv...)
	if args.Workdir != "" {
		cmd.Dir = args.Workdir
	}
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	runErr := cmd.Run()

	output := buf.String()
	if len(output) > maxToolOutput {
		output = output[:maxToolOutput] + "\n... (output truncated)"
	}

	switch {
	case runErr == nil:
		return "exit: 0\n" + output, nil
	case cctx.Err() == context.DeadlineExceeded:
		return fmt.Sprintf("exit: -1 (timed out after %d seconds)\n%s", timeout, output), nil
	default:
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Sprintf("exit: %d\n%s", exitErr.ExitCode(), output), nil
		}
		return "", fmt.Errorf("run command: %w", runErr)
	}
}

func shellProgram() string {
	if runtime.GOOS == "windows" {
		return "powershell.exe"
	}
	return "bash"
}

// posixismError rejects commands that cannot parse in Windows PowerShell 5.1,
// turning a cryptic parser error into an actionable one before execution.
func posixismError(command string) string {
	switch {
	case strings.Contains(command, "&&"):
		return "'&&' is not a valid operator in Windows PowerShell 5.1; chain dependent commands with '; if ($?) { ... }'"
	case strings.Contains(command, "||"):
		return "'||' is not a valid operator in Windows PowerShell 5.1; run the commands separately or use 'if (-not $?) { ... }'"
	case strings.Contains(command, "/dev/null"):
		return "'/dev/null' does not exist on Windows; discard output with '2>$null' or '| Out-Null'"
	}
	return ""
}
