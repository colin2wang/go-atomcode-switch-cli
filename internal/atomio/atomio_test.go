package atomio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-atomcode-switch-cli/internal/models"
)

func sampleAuth() *models.AuthToml {
	return &models.AuthToml{
		AccessToken:  "token",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		CreatedAt:    12345,
		User: models.User{
			ID:       "user-1",
			Username: "zhang.san",
			Name:     "张三",
			Email:    "zhang@example.com",
		},
	}
}

func TestReadReturnsNotFoundWhenMissing(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(dir); !IsNotExist(err) {
		t.Fatalf("Read() error = %v, want ErrAuthNotFound", err)
	}
}

func TestReadReturnsNotFoundWhenMalformed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(AuthPath(dir), []byte("[user\nid = \"x\""), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); !IsNotExist(err) {
		t.Fatalf("Read() error = %v, want ErrAuthNotFound", err)
	}
}

func TestWriteAndReadAuth(t *testing.T) {
	dir := t.TempDir()
	auth := sampleAuth()
	if err := Write(dir, auth); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.User.ID != auth.User.ID || got.AccessToken != auth.AccessToken {
		t.Fatalf("unexpected auth: %+v", got)
	}
	data, err := os.ReadFile(AuthPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "avatar_url") {
		t.Fatalf("empty avatar_url was written: %s", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "auth.toml.zhang.san")); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "auth.toml.bak")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected .bak file: %v", err)
	}
}

func TestSnapshotNameSanitizesCharacters(t *testing.T) {
	auth := sampleAuth()
	auth.User.Username = ""
	auth.User.Name = "a/b c@d"
	if got := snapshotName(auth); got != "a_b_c_d" {
		t.Fatalf("snapshotName() = %q, want a_b_c_d", got)
	}
}
