package plugindevtest

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// TestExternalSDKProject exercises the real CLI outside Kumbuka's module, then loads
// its package through the same manager and sanitizer used by installed plugins.
func TestExternalSDKProject(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	cli := filepath.Join(temp, "kumbuka-plugin")
	command := exec.Command("go", "build", "-o", cli, "github.com/kumbuka-me/sdk/cmd/kumbuka-plugin")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("CLI build: %v\n%s", err, output)
	}
	run := func(dir string, args ...string) {
		t.Helper()
		command := exec.Command(cli, args...)
		command.Dir = dir
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	run(temp, "init", "my-plugin")
	project := filepath.Join(temp, "my-plugin")
	// Include hostile HTML to prove SDK output still crosses Kumbuka's sanitizer.
	source, err := os.ReadFile(filepath.Join(project, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.ReplaceAll(source, []byte("<p>Hello"), []byte("<script>alert(1)</script><p>Hello"))
	if err := os.WriteFile(filepath.Join(project, "main.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	run(project, "test")
	run(project, "build")
	filename := filepath.Join(project, "dist", "my-plugin.kumbukaplugin")
	first, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	run(project, "build")
	second, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("build is not deterministic")
	}
	if _, err := pluginpackage.Read(first); err != nil {
		t.Fatal(err)
	}
	runtime, err := wasm.New(ctx, wasm.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	t.Cleanup(func() { _ = manager.Close(ctx) })
	metadata, err := manager.Load(ctx, first, plugin.SourceInstalled)
	if err != nil {
		t.Fatal(err)
	}
	renderer := markdown.NewWithRegistry(registry)
	html, err := renderer.Render("{{greeting}}\n\n```\n{{greeting}}\n```\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Hello from a Go plugin!") || !strings.Contains(html, "{{greeting}}") || strings.Contains(html, "<script") {
		t.Fatalf("unexpected rendered output: %s", html)
	}
	if err := manager.Unload(ctx, metadata.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	html, err = renderer.Render("{{greeting}}")
	if err != nil || strings.Contains(html, "Hello from") {
		t.Fatalf("unload: %s %v", html, err)
	}
	if _, err := manager.Load(ctx, first, plugin.SourceInstalled); err != nil {
		t.Fatal(err)
	}
	html, err = renderer.Render("{{greeting}}")
	if err != nil || !strings.Contains(html, "Hello from") {
		t.Fatalf("reload: %s %v", html, err)
	}
}
