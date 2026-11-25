package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	// ErrTimeout 表示获取登录信息超时。
	ErrTimeout = errors.New("fetch timeout")
	// ErrNoOutput 表示子进程没有产生任何输出。
	ErrNoOutput = errors.New("no output")
)

const pollInterval = 100 * time.Millisecond

var earlyKeywords = []string{
	"套餐", "Plan:", "剩余", "Usage", "usage", "到期时间", "resets", "重置于",
}

type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuf) ContainsAny(keywords ...string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.buf.String()
	for _, keyword := range keywords {
		if strings.Contains(s, keyword) {
			return true
		}
	}
	return false
}

// FetchLoginOutput 执行 atomcode login，捕获 stdout/stderr 并支持关键词早退与超时终止。
func FetchLoginOutput(parent context.Context, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "atomcode", "login")
	cmd.Stdin = nil
	var stdout, stderr syncBuf
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("exec atomcode login: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	exited := false
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-waitCh:
			exited = true
			break loop
		case <-tick.C:
			if stdout.ContainsAny(earlyKeywords...) || stderr.ContainsAny(earlyKeywords...) {
				break loop
			}
		}
	}

	if !exited {
		_ = cmd.Process.Kill()
		<-waitCh
	}

	combined := strings.TrimSpace(stdout.String())
	if e := strings.TrimSpace(stderr.String()); e != "" {
		if combined == "" {
			combined = e
		} else {
			combined += "\n" + e
		}
	}
	if combined == "" {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ErrTimeout
		}
		return "", ErrNoOutput
	}
	return combined, nil
}
