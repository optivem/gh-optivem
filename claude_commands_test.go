package main

import (
	"os"
	"path/filepath"
	"testing"

	claudeassets "github.com/optivem/gh-optivem/internal/claude/assets"
)

// writeEmbedded copies an embedded asset into claudeDir under destRel so the
// helper under test sees an "in sync" global file.
func writeEmbedded(t *testing.T, claudeDir, srcRel, destRel string) {
	t.Helper()
	data, err := claudeassets.FS.ReadFile(srcRel)
	if err != nil {
		t.Fatalf("read embedded %s: %v", srcRel, err)
	}
	dest := filepath.Join(claudeDir, destRel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dest, err)
	}
}

func TestClaudeSettingsDrift_MissingFile_IsDrift(t *testing.T) {
	claudeDir := t.TempDir()
	drift, err := claudeSettingsDrift(claudeDir)
	if err != nil {
		t.Fatalf("claudeSettingsDrift: %v", err)
	}
	if !drift {
		t.Errorf("missing settings.json should report drift")
	}
}

func TestClaudeSettingsDrift_EmbeddedCopy_InSync(t *testing.T) {
	claudeDir := t.TempDir()
	writeEmbedded(t, claudeDir, "config/settings.json", "settings.json")
	drift, err := claudeSettingsDrift(claudeDir)
	if err != nil {
		t.Fatalf("claudeSettingsDrift: %v", err)
	}
	if drift {
		t.Errorf("settings.json identical to embedded should report in sync")
	}
}

func TestClaudeSettingsDrift_EmptyObject_IsDrift(t *testing.T) {
	claudeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	drift, err := claudeSettingsDrift(claudeDir)
	if err != nil {
		t.Fatalf("claudeSettingsDrift: %v", err)
	}
	if !drift {
		t.Errorf("empty settings.json should report drift (embedded perms/hooks absent)")
	}
}

func TestClaudeMDMissingHeaders_MissingFile_AllSections(t *testing.T) {
	claudeDir := t.TempDir()
	headers, err := claudeMDMissingHeaders(claudeDir)
	if err != nil {
		t.Fatalf("claudeMDMissingHeaders: %v", err)
	}
	if len(headers) == 0 {
		t.Errorf("missing CLAUDE.md should report every embedded ## section as missing")
	}
}

func TestClaudeMDMissingHeaders_EmbeddedCopy_None(t *testing.T) {
	claudeDir := t.TempDir()
	writeEmbedded(t, claudeDir, "config/CLAUDE.md", "CLAUDE.md")
	headers, err := claudeMDMissingHeaders(claudeDir)
	if err != nil {
		t.Fatalf("claudeMDMissingHeaders: %v", err)
	}
	if len(headers) != 0 {
		t.Errorf("CLAUDE.md identical to embedded should report no missing sections, got %v", headers)
	}
}
