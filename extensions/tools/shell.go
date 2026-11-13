package tools

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	shellOnce sync.Once
	shellDesc string
)

// ShellDescription names the shell BashTool executes commands in, probing the
// PowerShell version once on Windows.
func ShellDescription() string {
	shellOnce.Do(func() {
		if runtime.GOOS != "windows" {
			shellDesc = "bash (invoked as: bash -c)"
			return
		}
		shellDesc = "Windows PowerShell, version unknown (invoked as: powershell.exe -NoProfile -NonInteractive -Command)"
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive",
			"-Command", "$PSVersionTable.PSVersion.ToString()").Output()
		if err != nil {
			return
		}
		if v := strings.TrimSpace(string(out)); v != "" {
			shellDesc = fmt.Sprintf("Windows PowerShell %s (invoked as: powershell.exe -NoProfile -NonInteractive -Command)", v)
		}
	})
	return shellDesc
}

// ShellGuidelines returns shell-specific do's and don'ts for the model.
func ShellGuidelines() string {
	if runtime.GOOS == "windows" {
		return "- Chain dependent commands with '; if ($?) { cmd2 }'. The '&&' and '||' operators do not exist in Windows PowerShell 5.1.\n" +
			"- Check exit codes with '$LASTEXITCODE'; '$?' is a boolean, not an exit code.\n" +
			"- There is no /dev/null. Discard output with '2>$null' or '| Out-Null'.\n" +
			"- Prefer full cmdlet names (Get-ChildItem, Set-Content, Remove-Item) over POSIX aliases."
	}
	return "- Standard bash syntax applies: '&&', '||', pipes, and '2>/dev/null' all work."
}
