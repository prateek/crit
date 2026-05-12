package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests cover the destination-routing rules for non-aider integrations.
// They protect against regressions in integrationMap entries: missing
// globalDest fields, wrong globalDestKind, or wrong destination paths.
// (The aider integration has its own end-to-end coverage in aider_install_test.go.)

func TestDestFor_ProjectMode(t *testing.T) {
	// Project install must always use the dest field, regardless of whether a
	// globalDest is set.
	for name, files := range integrationMap {
		for i, f := range files {
			got := destFor(f, false, "/home/me", name)
			if got != f.dest {
				t.Errorf("%s[%d] project: got %q, want %q", name, i, got, f.dest)
			}
		}
	}
}

func TestDestFor_GlobalMode(t *testing.T) {
	// In global mode, integrations with no globalDest fall through to the raw
	// relative dest (which lands under $HOME because cwd == $HOME during a
	// global install). Integrations with a globalDest get the redirected
	// absolute path. The two routes have meaningfully different semantics, so
	// the test exercises both.
	home := "/home/me"
	cases := []struct {
		tool    string
		fileIdx int
		want    string
	}{
		// No globalDest → raw relative dest, written cwd-relative (cwd == $HOME).
		{"claude-code", 0, ".claude/skills/crit/SKILL.md"},
		{"claude-code", 1, ".claude/skills/crit-cli/SKILL.md"},
		{"codex", 0, ".agents/skills/crit/SKILL.md"},
		{"codex", 1, ".agents/skills/crit-cli/SKILL.md"},
		{"qwen", 0, ".qwen/skills/crit/SKILL.md"},
		{"qwen", 1, ".qwen/skills/crit-cli/SKILL.md"},
		{"cursor", 0, ".cursor/skills/crit/SKILL.md"},
		{"cursor", 1, ".cursor/skills/crit-cli/SKILL.md"},
		// opencode: command stays cwd-relative; skill redirects globally to ~/.agents/skills/.
		{"opencode", 0, ".opencode/commands/crit.md"},
		{"opencode", 1, filepath.Join(home, ".agents/skills/crit/SKILL.md")},
		// github-copilot: both skills redirect to ~/.agents/skills/.
		{"github-copilot", 0, filepath.Join(home, ".agents/skills/crit/SKILL.md")},
		{"github-copilot", 1, filepath.Join(home, ".agents/skills/crit-cli/SKILL.md")},
		// hermes: both skills redirect to ~/.hermes/skills/.
		{"hermes", 0, filepath.Join(home, ".hermes/skills/crit/SKILL.md")},
		{"hermes", 1, filepath.Join(home, ".hermes/skills/crit-cli/SKILL.md")},
		// pi: both skills redirect to ~/.pi/agent/skills/.
		{"pi", 0, filepath.Join(home, ".pi/agent/skills/crit/SKILL.md")},
		{"pi", 1, filepath.Join(home, ".pi/agent/skills/crit-cli/SKILL.md")},
	}
	for _, tc := range cases {
		f := integrationMap[tc.tool][tc.fileIdx]
		got := destFor(f, true, home, tc.tool)
		if got != tc.want {
			t.Errorf("%s[%d] global: got %q, want %q", tc.tool, tc.fileIdx, got, tc.want)
		}
	}
}

func TestDestFor_ClineGlobalUsesDocuments(t *testing.T) {
	// Cline's globalDest uses the platform Documents directory, not $HOME directly.
	prev := xdgUserDirFn
	t.Cleanup(func() { xdgUserDirFn = prev })
	xdgUserDirFn = func(string) (string, error) { return "", nil }

	home := "/home/me"
	f := integrationMap["cline"][0]
	got := destFor(f, true, home, "cline")
	want := filepath.Join(documentsDir(home), "Cline/Rules/crit.md")
	if got != want {
		t.Errorf("cline global: got %q, want %q", got, want)
	}
	// On non-Linux, this should always be $HOME/Documents/Cline/Rules/crit.md.
	if runtime.GOOS != "linux" {
		expected := filepath.Join(home, "Documents/Cline/Rules/crit.md")
		if got != expected {
			t.Errorf("cline global on %s: got %q, want %q", runtime.GOOS, got, expected)
		}
	}
}

func TestIntegrationMap_SnapshotGlobalRouting(t *testing.T) {
	// Snapshot test: verifies each tool's globalDest configuration matches
	// what the integration validation findings established. Update this test
	// when intentionally changing routing.
	type want struct {
		globalDest string
		kind       globalDestKind
	}
	expected := map[string][]want{
		"claude-code":    {{"", globalDestNone}, {"", globalDestNone}},
		"cursor":         {{"", globalDestNone}, {"", globalDestNone}},
		"codex":          {{"", globalDestNone}, {"", globalDestNone}},
		"qwen":           {{"", globalDestNone}, {"", globalDestNone}},
		"opencode":       {{"", globalDestNone}, {".agents/skills/crit/SKILL.md", globalDestRelHome}, {".config/opencode/plugins/crit.ts", globalDestRelHome}},
		"github-copilot": {{".agents/skills/crit/SKILL.md", globalDestRelHome}, {".agents/skills/crit-cli/SKILL.md", globalDestRelHome}},
		"windsurf":       {{"", globalDestNone}},
		"cline":          {{"Cline/Rules/crit.md", globalDestDocuments}},
		"gemini":         {{".gemini/skills/crit-cli/SKILL.md", globalDestRelHome}, {".gemini/commands/crit.toml", globalDestRelHome}, {".gemini/policies/crit.toml", globalDestRelHome}},
		"codex-plugin":   {{".codex/plugins/crit/.codex-plugin/plugin.json", globalDestRelHome}, {".codex/plugins/crit/skills/crit/SKILL.md", globalDestRelHome}, {".codex/plugins/crit/skills/crit-cli/SKILL.md", globalDestRelHome}, {".codex/plugins/crit/hooks/hooks.json", globalDestRelHome}},
		"hermes":         {{".hermes/skills/crit/SKILL.md", globalDestRelHome}, {".hermes/skills/crit-cli/SKILL.md", globalDestRelHome}},
		"pi":             {{".pi/agent/skills/crit/SKILL.md", globalDestRelHome}, {".pi/agent/skills/crit-cli/SKILL.md", globalDestRelHome}},
	}
	for tool, files := range expected {
		got := integrationMap[tool]
		if len(got) != len(files) {
			t.Errorf("%s: got %d files, want %d", tool, len(got), len(files))
			continue
		}
		for i, w := range files {
			if got[i].globalDest != w.globalDest || got[i].globalDestKind != w.kind {
				t.Errorf("%s[%d]: got (%q, kind=%d), want (%q, kind=%d)",
					tool, i, got[i].globalDest, got[i].globalDestKind, w.globalDest, w.kind)
			}
		}
	}
}

func TestInstallOneFile_WritesAndSkips(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "subdir", "out.md")
	f := integration{source: "integrations/cline/crit.md", dest: dest}

	// First install: file written.
	installOneFile(f, dest, false)
	if _, err := os.ReadFile(dest); err != nil {
		t.Fatalf("expected file at %s: %v", dest, err)
	}

	// Second install without --force: should skip without erroring.
	// Modify the file to verify it's not overwritten.
	if err := os.WriteFile(dest, []byte("hand-edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	installOneFile(f, dest, false)
	got, _ := os.ReadFile(dest)
	if string(got) != "hand-edited" {
		t.Errorf("non-force should skip; file was overwritten: %q", got)
	}

	// Force install: file overwritten with embedded content.
	installOneFile(f, dest, true)
	got, _ = os.ReadFile(dest)
	if string(got) == "hand-edited" {
		t.Errorf("force should overwrite; file still has hand-edited content")
	}
}

// TestInstallIntegration_GeminiWritesSettingsJSON verifies that the gemini
// special-case in installIntegration runs installGeminiSettings and produces
// a .gemini/settings.json in the project directory.
func TestInstallIntegration_GeminiWritesSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := installIntegration("gemini", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}
	settingsPath := filepath.Join(dir, ".gemini", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected .gemini/settings.json to be written: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	hooks, _ := m["hooks"].(map[string]interface{})
	before, _ := hooks["BeforeTool"].([]interface{})
	for _, e := range before {
		if em, ok := e.(map[string]interface{}); ok && em["matcher"] == "exit_plan_mode" {
			return
		}
	}
	t.Error("exit_plan_mode hook not found in .gemini/settings.json")
}

func TestInstallIntegration_CodexPluginEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}

	for _, path := range []string{
		".agents/skills/crit/SKILL.md",
		".agents/skills/crit-cli/SKILL.md",
		"plugins/crit/.codex-plugin/plugin.json",
		"plugins/crit/skills/crit/SKILL.md",
		"plugins/crit/skills/crit-cli/SKILL.md",
		"plugins/crit/hooks/hooks.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to be written: %v", path, err)
		}
	}

	hookPath := filepath.Join(dir, "plugins/crit/hooks/hooks.json")
	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hookData), "crit plan-hook --mode codex") {
		t.Fatalf("plugin hook should invoke crit plan-hook --mode codex:\n%s", hookData)
	}

	marketplacePath := filepath.Join(dir, ".agents/plugins/marketplace.json")
	if got := countCritMarketplaceEntries(t, marketplacePath, "./plugins/crit"); got != 1 {
		t.Fatalf("expected one Crit marketplace entry after first install, got %d", got)
	}
	assertCritMarketplacePathExists(t, marketplacePath, dir)

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("second installIntegration: %v", err)
	}
	if got := countCritMarketplaceEntries(t, marketplacePath, "./plugins/crit"); got != 1 {
		t.Fatalf("expected idempotent marketplace registration, got %d entries", got)
	}
}

func TestInstallIntegration_CodexPluginGlobalEndToEnd(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	t.Chdir(home)

	if err := installIntegration("codex-plugin", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}

	for _, path := range []string{
		".agents/skills/crit/SKILL.md",
		".agents/skills/crit-cli/SKILL.md",
		".codex/plugins/crit/.codex-plugin/plugin.json",
		".codex/plugins/crit/skills/crit/SKILL.md",
		".codex/plugins/crit/skills/crit-cli/SKILL.md",
		".codex/plugins/crit/hooks/hooks.json",
	} {
		if _, err := os.Stat(filepath.Join(home, path)); err != nil {
			t.Fatalf("expected %s to be written: %v", path, err)
		}
	}

	marketplacePath := filepath.Join(home, ".agents/plugins/marketplace.json")
	if got := countCritMarketplaceEntries(t, marketplacePath, "./.codex/plugins/crit"); got != 1 {
		t.Fatalf("expected one Crit marketplace entry after global install, got %d", got)
	}
	assertCritMarketplacePathExists(t, marketplacePath, home)
}

func TestInstallCodexPluginMarketplaceForceOverwritesInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	installCodexPluginMarketplace(path, "./plugins/crit", true)

	if got := countCritMarketplaceEntries(t, path, "./plugins/crit"); got != 1 {
		t.Fatalf("expected one Crit marketplace entry, got %d", got)
	}
}

func TestInstallCodexPluginMarketplaceForceOverwritesMalformedPlugins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	if err := os.WriteFile(path, []byte(`{"plugins":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	installCodexPluginMarketplace(path, "./plugins/crit", true)

	if got := countCritMarketplaceEntries(t, path, "./plugins/crit"); got != 1 {
		t.Fatalf("expected one Crit marketplace entry, got %d", got)
	}
}

func TestInstallCodexPluginMarketplaceRepairsStaleSourcePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "marketplace.json")
	stale := `{
  "name": "local",
  "plugins": [
    {
      "name": "crit",
      "source": {"source": "local", "path": "./plugins/crit"},
      "policy": {"installation": "INSTALLED_BY_DEFAULT", "authentication": "ON_INSTALL"},
      "category": "Developer Tools"
    }
  ]
}`
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	installCodexPluginMarketplace(path, "./.codex/plugins/crit", false)

	if got := countCritMarketplaceEntries(t, path, "./.codex/plugins/crit"); got != 1 {
		t.Fatalf("expected stale Crit marketplace entry to be replaced, got %d", got)
	}
}

func countCritMarketplaceEntries(t *testing.T, path, wantSourcePath string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected marketplace at %s: %v", path, err)
	}
	var marketplace map[string]interface{}
	if err := json.Unmarshal(data, &marketplace); err != nil {
		t.Fatalf("marketplace is not valid JSON: %v", err)
	}
	plugins, _ := marketplace["plugins"].([]interface{})
	count := 0
	for _, raw := range plugins {
		plugin, _ := raw.(map[string]interface{})
		if plugin["name"] != "crit" {
			continue
		}
		count++
		source, _ := plugin["source"].(map[string]interface{})
		if source["source"] != "local" || source["path"] != wantSourcePath {
			t.Fatalf("unexpected Crit plugin source: %+v", source)
		}
		policy, _ := plugin["policy"].(map[string]interface{})
		if policy["installation"] != "INSTALLED_BY_DEFAULT" {
			t.Fatalf("unexpected Crit plugin policy: %+v", policy)
		}
	}
	return count
}

func assertCritMarketplacePathExists(t *testing.T, marketplacePath, marketplaceRoot string) {
	t.Helper()

	data, err := os.ReadFile(marketplacePath)
	if err != nil {
		t.Fatal(err)
	}
	var marketplace map[string]interface{}
	if err := json.Unmarshal(data, &marketplace); err != nil {
		t.Fatal(err)
	}
	plugins, _ := marketplace["plugins"].([]interface{})
	for _, raw := range plugins {
		plugin, _ := raw.(map[string]interface{})
		if plugin["name"] != "crit" {
			continue
		}
		source, _ := plugin["source"].(map[string]interface{})
		relPath, _ := source["path"].(string)
		pluginRoot := filepath.Join(marketplaceRoot, relPath)
		manifestPath := filepath.Join(pluginRoot, ".codex-plugin", "plugin.json")
		if _, err := os.Stat(manifestPath); err != nil {
			t.Fatalf("marketplace path %q should resolve to installed plugin manifest %s: %v", relPath, manifestPath, err)
		}
		return
	}
	t.Fatal("Crit marketplace entry not found")
}

// TestInstallIntegration_HermesPrintsExternalDirsNote verifies that on a
// project-mode install, the hermes special-case prints the external_dirs
// guidance — Hermes does not auto-discover project-local skills, so the
// note is the only thing that makes the project-install path useful.
func TestInstallIntegration_HermesPrintsExternalDirsNote(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	prev := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = prev })

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	if err := installIntegration("hermes", false); err != nil {
		t.Fatalf("installIntegration: %v", err)
	}
	_ = w.Close()
	out := <-done

	if _, err := os.Stat(filepath.Join(dir, ".hermes/skills/crit/SKILL.md")); err != nil {
		t.Fatalf("expected .hermes/skills/crit/SKILL.md to be written: %v", err)
	}
	for _, want := range []string{"~/.hermes/skills/", "external_dirs", "config.yaml"} {
		if !strings.Contains(out, want) {
			t.Errorf("project install output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestPrintUniqueHints_Dedups(t *testing.T) {
	// printUniqueHints prints to stdout; we just verify it doesn't panic on
	// duplicates and empty input. Output ordering and dedup logic are simple
	// enough that visual inspection during integration use covers the rest.
	printUniqueHints(nil)
	printUniqueHints([]string{"a", "b", "a", "c", "b"})
}

func TestInstallGeminiSettings(t *testing.T) {
	hookEntry := func(data []byte) bool {
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			return false
		}
		hooks, _ := m["hooks"].(map[string]interface{})
		before, _ := hooks["BeforeTool"].([]interface{})
		for _, e := range before {
			em, ok := e.(map[string]interface{})
			if ok && em["matcher"] == "exit_plan_mode" {
				return true
			}
		}
		return false
	}

	t.Run("creates file with hook when absent", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		installGeminiSettings(path, false)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected settings.json to be written: %v", err)
		}
		if !hookEntry(data) {
			t.Errorf("exit_plan_mode hook not found in %s", data)
		}
	})

	t.Run("skips when hook already present and no force", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		// Write a valid settings.json that already has the hook plus a sentinel field.
		prebuilt := `{"hooks":{"BeforeTool":[{"matcher":"exit_plan_mode","hooks":[{"type":"command","command":"crit plan-hook","timeout":345600000}]}]},"sentinel":true}` + "\n"
		_ = os.WriteFile(path, []byte(prebuilt), 0o644)
		installGeminiSettings(path, false)
		got, _ := os.ReadFile(path)
		if string(got) != prebuilt {
			t.Errorf("no-force should skip; file was modified:\ngot:  %s\nwant: %s", got, prebuilt)
		}
	})

	t.Run("force overwrites existing hook entry", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		// write a settings file with a stale exit_plan_mode entry
		stale := `{"hooks":{"BeforeTool":[{"matcher":"exit_plan_mode","hooks":[{"type":"command","command":"old-cmd","timeout":1}]}]}}`
		_ = os.WriteFile(path, []byte(stale), 0o644)
		installGeminiSettings(path, true)
		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		_ = json.Unmarshal(data, &m)
		hooks, _ := m["hooks"].(map[string]interface{})
		before, _ := hooks["BeforeTool"].([]interface{})
		// exactly one exit_plan_mode entry
		count := 0
		for _, e := range before {
			em, ok := e.(map[string]interface{})
			if ok && em["matcher"] == "exit_plan_mode" {
				count++
				inner, _ := em["hooks"].([]interface{})
				if len(inner) > 0 {
					cmd, _ := inner[0].(map[string]interface{})["command"].(string)
					if cmd == "old-cmd" {
						t.Error("stale command not replaced")
					}
				}
			}
		}
		if count != 1 {
			t.Errorf("expected exactly 1 exit_plan_mode entry, got %d", count)
		}
	})

	t.Run("preserves existing unrelated hooks", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.json")
		existing := `{"hooks":{"BeforeTool":[{"matcher":"other_tool","hooks":[{"type":"command","command":"other"}]}]}}`
		_ = os.WriteFile(path, []byte(existing), 0o644)
		installGeminiSettings(path, false)
		data, _ := os.ReadFile(path)
		var m map[string]interface{}
		_ = json.Unmarshal(data, &m)
		hooks, _ := m["hooks"].(map[string]interface{})
		before, _ := hooks["BeforeTool"].([]interface{})
		hasOther, hasCrit := false, false
		for _, e := range before {
			em, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			switch em["matcher"] {
			case "other_tool":
				hasOther = true
			case "exit_plan_mode":
				hasCrit = true
			}
		}
		if !hasOther {
			t.Error("pre-existing other_tool hook was removed")
		}
		if !hasCrit {
			t.Error("exit_plan_mode hook not added")
		}
	})
}
