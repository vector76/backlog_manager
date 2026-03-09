package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vector76/backlog_manager/internal/config"
)

func writeConfig(t *testing.T, dir string, cfg map[string]any) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":               8080,
		"data_dir":           "./data",
		"dashboard_user":     "admin",
		"dashboard_password": "changeme",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Port)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("expected data_dir ./data, got %q", cfg.DataDir)
	}
	if cfg.DashboardUser != "admin" {
		t.Errorf("expected dashboard_user admin, got %q", cfg.DashboardUser)
	}
	if cfg.DashboardPassword != "changeme" {
		t.Errorf("expected dashboard_password changeme, got %q", cfg.DashboardPassword)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_MissingPort(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"data_dir":           "./data",
		"dashboard_user":     "admin",
		"dashboard_password": "changeme",
	})
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing port")
	}
}

func TestLoad_MissingDataDir(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":               8080,
		"dashboard_user":     "admin",
		"dashboard_password": "changeme",
	})
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing data_dir")
	}
}

func TestLoad_MissingDashboardUser(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":               8080,
		"data_dir":           "./data",
		"dashboard_password": "changeme",
	})
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing dashboard_user")
	}
}

func TestLoad_ViewerFieldsAbsent(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":               8080,
		"data_dir":           "./data",
		"dashboard_user":     "admin",
		"dashboard_password": "changeme",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ViewerUser != "" {
		t.Errorf("expected empty viewer_user, got %q", cfg.ViewerUser)
	}
	if cfg.ViewerPassword != "" {
		t.Errorf("expected empty viewer_password, got %q", cfg.ViewerPassword)
	}
}

func TestLoad_ViewerFieldsPresent(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":               8080,
		"data_dir":           "./data",
		"dashboard_user":     "admin",
		"dashboard_password": "changeme",
		"viewer_user":        "viewer",
		"viewer_password":    "viewpass",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ViewerUser != "viewer" {
		t.Errorf("expected viewer_user viewer, got %q", cfg.ViewerUser)
	}
	if cfg.ViewerPassword != "viewpass" {
		t.Errorf("expected viewer_password viewpass, got %q", cfg.ViewerPassword)
	}
}

func TestLoad_ViewerFieldsPartial(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":               8080,
		"data_dir":           "./data",
		"dashboard_user":     "admin",
		"dashboard_password": "changeme",
		"viewer_user":        "viewer",
	})
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ViewerUser != "viewer" {
		t.Errorf("expected viewer_user viewer, got %q", cfg.ViewerUser)
	}
	if cfg.ViewerPassword != "" {
		t.Errorf("expected empty viewer_password, got %q", cfg.ViewerPassword)
	}
}

func TestLoad_MissingDashboardPassword(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, map[string]any{
		"port":           8080,
		"data_dir":       "./data",
		"dashboard_user": "admin",
	})
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing dashboard_password")
	}
}
