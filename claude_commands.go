// claude_commands.go implements `gh optivem claude` subcommands for managing
// Claude Code commands and configuration globally.
//
// install   — writes embedded command files to ~/.claude/commands/
// configure — merges embedded settings.json and CLAUDE.md into ~/.claude/ non-destructively
// setup     — runs install then configure
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	claudeassets "github.com/optivem/gh-optivem/internal/claude/assets"
)

func newClaudeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Manage global Claude Code commands and configuration",
	}
	cmd.AddCommand(
		newClaudeInstallCmd(),
		newClaudeConfigureCmd(),
		newClaudeSetupCmd(),
	)
	return cmd
}

func newClaudeInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install Optivem Claude commands to ~/.claude/commands/",
		Long: `Copy the embedded Optivem Claude slash commands into ~/.claude/commands/.

Files already up to date are skipped. Changed files are overwritten.
New files are added. Prints a summary of what changed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClaudeInstall()
		},
	}
}

func newClaudeConfigureCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "configure",
		Short: "Merge Optivem settings and CLAUDE.md rules into ~/.claude/ (non-destructive)",
		Long: `Merge Optivem Claude configuration into your global ~/.claude/ directory.

settings.json: unions the permissions.allow list and adds any hook event
types not already present — never removes entries you have set yourself.

CLAUDE.md: appends sections not already present in your ~/.claude/CLAUDE.md —
never overwrites or removes existing content.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClaudeConfigure()
		},
	}
}

func newClaudeSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Install commands and merge configuration (runs install then configure)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runClaudeInstall(); err != nil {
				return err
			}
			return runClaudeConfigure()
		},
	}
}

func runClaudeInstall() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	destDir := filepath.Join(home, ".claude", "commands")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", destDir, err)
	}

	entries, err := fs.ReadDir(claudeassets.FS, "commands")
	if err != nil {
		return fmt.Errorf("read embedded commands: %w", err)
	}

	added, updated, skipped := 0, 0, 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := fs.ReadFile(claudeassets.FS, "commands/"+entry.Name())
		if err != nil {
			return fmt.Errorf("read embedded command %s: %w", entry.Name(), err)
		}
		dest := filepath.Join(destDir, entry.Name())
		existing, readErr := os.ReadFile(dest)
		switch {
		case readErr == nil && bytes.Equal(existing, content):
			skipped++
		case readErr == nil:
			if err := os.WriteFile(dest, content, 0644); err != nil {
				return fmt.Errorf("write %s: %w", dest, err)
			}
			fmt.Printf("  Updated: %s\n", entry.Name())
			updated++
		default:
			if err := os.WriteFile(dest, content, 0644); err != nil {
				return fmt.Errorf("write %s: %w", dest, err)
			}
			fmt.Printf("  Added:   %s\n", entry.Name())
			added++
		}
	}
	fmt.Printf("Commands: %d added, %d updated, %d already up to date\n", added, updated, skipped)
	return nil
}

func runClaudeConfigure() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", claudeDir, err)
	}
	if err := mergeClaudeSettings(claudeDir); err != nil {
		return err
	}
	return mergeClaudeMD(claudeDir)
}

func mergeClaudeSettings(claudeDir string) error {
	embeddedData, err := fs.ReadFile(claudeassets.FS, "config/settings.json")
	if err != nil {
		return fmt.Errorf("read embedded settings.json: %w", err)
	}
	var embedded map[string]interface{}
	if err := json.Unmarshal(embeddedData, &embedded); err != nil {
		return fmt.Errorf("parse embedded settings.json: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	existingData, readErr := os.ReadFile(settingsPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", settingsPath, readErr)
	}

	var user map[string]interface{}
	if readErr == nil {
		if err := json.Unmarshal(existingData, &user); err != nil {
			return fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	} else {
		user = map[string]interface{}{}
	}

	changed := claudeSettingsMerge(user, embedded)
	if !changed && readErr == nil {
		fmt.Println("settings.json: already up to date")
		return nil
	}

	out, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings.json: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0644); err != nil {
		return fmt.Errorf("write %s: %w", settingsPath, err)
	}
	if readErr == nil {
		fmt.Println("settings.json: updated")
	} else {
		fmt.Println("settings.json: created")
	}
	return nil
}

// claudeSettingsMerge unions permissions.allow entries and adds missing hook
// event keys from src into dst. Returns true if dst was modified.
func claudeSettingsMerge(dst, src map[string]interface{}) bool {
	changed := false

	// Union permissions.allow
	srcPerms, _ := src["permissions"].(map[string]interface{})
	srcAllow, _ := srcPerms["allow"].([]interface{})
	if len(srcAllow) > 0 {
		dstPerms, _ := dst["permissions"].(map[string]interface{})
		if dstPerms == nil {
			dstPerms = map[string]interface{}{}
			dst["permissions"] = dstPerms
		}
		dstAllow, _ := dstPerms["allow"].([]interface{})
		seen := make(map[string]bool, len(dstAllow))
		for _, v := range dstAllow {
			if s, ok := v.(string); ok {
				seen[s] = true
			}
		}
		for _, v := range srcAllow {
			if s, ok := v.(string); ok && !seen[s] {
				dstAllow = append(dstAllow, s)
				seen[s] = true
				changed = true
			}
		}
		if changed {
			dstPerms["allow"] = dstAllow
		}
	}

	// Add hook event types not already present
	srcHooks, _ := src["hooks"].(map[string]interface{})
	if len(srcHooks) > 0 {
		dstHooks, _ := dst["hooks"].(map[string]interface{})
		if dstHooks == nil {
			dstHooks = map[string]interface{}{}
			dst["hooks"] = dstHooks
		}
		for event, entry := range srcHooks {
			if _, ok := dstHooks[event]; !ok {
				dstHooks[event] = entry
				changed = true
			}
		}
	}

	return changed
}

func mergeClaudeMD(claudeDir string) error {
	embeddedData, err := fs.ReadFile(claudeassets.FS, "config/CLAUDE.md")
	if err != nil {
		return fmt.Errorf("read embedded CLAUDE.md: %w", err)
	}

	mdPath := filepath.Join(claudeDir, "CLAUDE.md")
	existingData, readErr := os.ReadFile(mdPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", mdPath, readErr)
	}

	userContent := string(existingData) // empty string when file doesn't exist
	toAppend := claudeMDMissingSections(userContent, string(embeddedData))
	if len(toAppend) == 0 {
		fmt.Println("CLAUDE.md: already up to date")
		return nil
	}

	var buf strings.Builder
	buf.WriteString(userContent)
	if userContent != "" && !strings.HasSuffix(userContent, "\n") {
		buf.WriteByte('\n')
	}
	if userContent != "" {
		buf.WriteByte('\n')
	}
	for i, section := range toAppend {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(section)
		if !strings.HasSuffix(section, "\n") {
			buf.WriteByte('\n')
		}
	}

	if err := os.WriteFile(mdPath, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", mdPath, err)
	}
	if readErr == nil {
		fmt.Printf("CLAUDE.md: appended %d section(s)\n", len(toAppend))
	} else {
		fmt.Printf("CLAUDE.md: created with %d section(s)\n", len(toAppend))
	}
	return nil
}

// claudeMDMissingSections returns the ## sections from src that are not
// already present in dst, matched by the ## header line.
func claudeMDMissingSections(dst, src string) []string {
	dstHeaders := make(map[string]bool)
	for _, line := range strings.Split(dst, "\n") {
		if strings.HasPrefix(line, "## ") {
			dstHeaders[strings.TrimSpace(line)] = true
		}
	}

	var sections []string
	var current strings.Builder
	var currentHeader string

	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "## ") {
			if currentHeader != "" {
				sec := strings.TrimRight(current.String(), "\n")
				if !dstHeaders[currentHeader] {
					sections = append(sections, sec)
				}
				current.Reset()
			}
			currentHeader = strings.TrimSpace(line)
		}
		if currentHeader != "" {
			current.WriteString(line)
			current.WriteByte('\n')
		}
	}
	if currentHeader != "" && current.Len() > 0 {
		sec := strings.TrimRight(current.String(), "\n")
		if !dstHeaders[currentHeader] {
			sections = append(sections, sec)
		}
	}
	return sections
}
