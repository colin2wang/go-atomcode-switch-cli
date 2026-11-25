package atomio

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/store"

	"github.com/BurntSushi/toml"
)

// ErrAuthNotFound 统一表示当前 auth.toml 不存在、不可读或格式损坏。
var ErrAuthNotFound = errors.New("auth.toml not found")

// Dir 返回 auth.toml 所在目录。
func Dir(custom string) string {
	if custom != "" {
		return custom
	}
	return filepath.Join(store.HomeDir(), ".atomcode")
}

// AuthPath 返回 auth.toml 的完整路径。
func AuthPath(custom string) string {
	return filepath.Join(Dir(custom), "auth.toml")
}

// Read 读取当前 auth.toml；任何失败都归一化为 ErrAuthNotFound。
func Read(custom string) (*models.AuthToml, error) {
	data, err := os.ReadFile(AuthPath(custom))
	if err != nil {
		return nil, ErrAuthNotFound
	}
	var auth models.AuthToml
	if _, err := toml.Decode(string(data), &auth); err != nil {
		slog.Debug("auth.toml decode failed", "err", err)
		return nil, ErrAuthNotFound
	}
	return &auth, nil
}

// Write 原子写 auth.toml，并 best-effort 写入按用户名命名的快照。
func Write(custom string, auth *models.AuthToml) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(auth); err != nil {
		return err
	}
	content := buf.String()
	if err := store.AtomicWriteText(AuthPath(custom), content, false); err != nil {
		return err
	}
	if name := snapshotName(auth); name != "" {
		snapshot := filepath.Join(Dir(custom), "auth.toml."+name)
		if err := store.AtomicWriteText(snapshot, content, false); err != nil {
			slog.Warn("auth snapshot failed", "path", snapshot, "err", err)
		}
	}
	return nil
}

// IsNotExist 判断错误是否表示当前 auth 不可用。
func IsNotExist(err error) bool {
	return errors.Is(err, ErrAuthNotFound)
}

func snapshotName(auth *models.AuthToml) string {
	raw := auth.User.Username
	if raw == "" {
		raw = auth.User.Name
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
