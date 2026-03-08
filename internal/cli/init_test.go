package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vector76/backlog_manager/internal/cli"
)

func runInitCmd(t *testing.T, input string, extraArgs ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	outPath := filepath.Join(dir, "config.json")
	args := append([]string{"init", "--output", outPath}, extraArgs...)

	var out bytes.Buffer
	root := cli.NewRootCmd()
	root.SetArgs(args)
	root.SetIn(strings.NewReader(input))
	root.SetOut(&out)
	err := root.Execute()
	return outPath, err
}

func readInitConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON in config: %v", err)
	}
	return cfg
}

func TestInitCmd_Defaults(t *testing.T) {
	// Port: default, Data dir: default, User: default, Password: "secret", Beads URL: skip
	outPath, err := runInitCmd(t, "\n\n\nsecret\n\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := readInitConfig(t, outPath)

	if cfg["port"] != float64(8080) {
		t.Errorf("expected port 8080, got %v", cfg["port"])
	}
	if cfg["dashboard_user"] != "admin" {
		t.Errorf("expected dashboard_user admin, got %v", cfg["dashboard_user"])
	}
	if cfg["dashboard_password"] != "secret" {
		t.Errorf("expected dashboard_password secret, got %v", cfg["dashboard_password"])
	}
	if cfg["beads_server_url"] != "" {
		t.Errorf("expected empty beads_server_url, got %v", cfg["beads_server_url"])
	}
	if cfg["data_dir"] == "" {
		t.Error("expected non-empty data_dir")
	}
}

func TestInitCmd_CustomValues(t *testing.T) {
	// Port: 9090, Data dir: /tmp/mydata, User: superuser, Password: p@ssw0rd, Beads URL: http://beads:9999
	input := "9090\n/tmp/mydata\nsuperuser\np@ssw0rd\nhttp://beads:9999\n"
	outPath, err := runInitCmd(t, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := readInitConfig(t, outPath)

	if cfg["port"] != float64(9090) {
		t.Errorf("expected port 9090, got %v", cfg["port"])
	}
	if cfg["data_dir"] != "/tmp/mydata" {
		t.Errorf("expected data_dir /tmp/mydata, got %v", cfg["data_dir"])
	}
	if cfg["dashboard_user"] != "superuser" {
		t.Errorf("expected dashboard_user superuser, got %v", cfg["dashboard_user"])
	}
	if cfg["dashboard_password"] != "p@ssw0rd" {
		t.Errorf("expected dashboard_password p@ssw0rd, got %v", cfg["dashboard_password"])
	}
	if cfg["beads_server_url"] != "http://beads:9999" {
		t.Errorf("expected beads_server_url http://beads:9999, got %v", cfg["beads_server_url"])
	}
}

func TestInitCmd_InvalidPort(t *testing.T) {
	_, err := runInitCmd(t, "notaport\n\n\nsecret\n\n")
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
	if !strings.Contains(err.Error(), "invalid port") {
		t.Errorf("expected 'invalid port' in error, got: %v", err)
	}
}

func TestInitCmd_EmptyPasswordRetries(t *testing.T) {
	// Port: default, Data dir: default, User: default, Password: empty then "mypass", Beads URL: skip
	var out bytes.Buffer
	dir := t.TempDir()
	outPath := filepath.Join(dir, "config.json")

	root := cli.NewRootCmd()
	root.SetArgs([]string{"init", "--output", outPath})
	root.SetIn(strings.NewReader("\n\n\n\nmypass\n\n"))
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Password is required.") {
		t.Errorf("expected retry message, got: %s", output)
	}

	cfg := readInitConfig(t, outPath)
	if cfg["dashboard_password"] != "mypass" {
		t.Errorf("expected dashboard_password mypass, got %v", cfg["dashboard_password"])
	}
}

func TestInitCmd_EmptyPasswordEOF(t *testing.T) {
	// EOF with no password supplied should return an error
	_, err := runInitCmd(t, "\n\n\n")
	if err == nil {
		t.Fatal("expected error when password not provided before EOF")
	}
	if !strings.Contains(err.Error(), "password is required") {
		t.Errorf("expected 'password is required' in error, got: %v", err)
	}
}

func TestInitCmd_FilePermissions(t *testing.T) {
	outPath, err := runInitCmd(t, "\n\n\nsecret\n\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %v", info.Mode().Perm())
	}
}

func TestInitCmd_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "config.json")
	// Pre-create the file
	if err := os.WriteFile(outPath, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	root := cli.NewRootCmd()
	root.SetArgs([]string{"init", "--output", outPath})
	var out bytes.Buffer
	root.SetOut(&out)
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error when output file already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

func TestInitCmd_RegisteredOnRoot(t *testing.T) {
	root := cli.NewRootCmd()
	names := make(map[string]bool)
	for _, c := range root.Commands() {
		names[c.Use] = true
	}
	if !names["init"] {
		t.Error("expected 'init' subcommand on root")
	}
}
