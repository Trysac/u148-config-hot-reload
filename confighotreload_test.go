package confighotreload_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Trysac/u148-config-hot-reload"
)

type Config struct {
	Port int
}

type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{files: make(map[string][]byte)}
}

func (m *memFS) set(path, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = []byte(content)
}

func (m *memFS) setBytes(path string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = content
}

func (m *memFS) remove(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, path)
}

func (m *memFS) open(path string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func parseConfig(data []byte) (Config, error) {
	raw := string(data)
	if !strings.HasPrefix(raw, "port=") {
		return Config{}, errors.New("missing port= prefix")
	}
	digits := strings.TrimPrefix(raw, "port=")
	p, err := strconv.Atoi(digits)
	if err != nil {
		return Config{}, err
	}
	return Config{Port: p}, nil
}

func validateConfig(cfg Config) error {
	if cfg.Port < 1 || cfg.Port > 65535 {
		return errors.New("port out of range 1-65535")
	}
	return nil
}

func TestInvalidOptions(t *testing.T) {
	_, err := confighotreload.New(confighotreload.Options[Config]{
		Path: "",
	})
	if !errors.Is(err, confighotreload.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions on empty path, got %v", err)
	}

	_, err = confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Parse: nil,
	})
	if !errors.Is(err, confighotreload.ErrInvalidOptions) {
		t.Fatalf("expected ErrInvalidOptions on nil Parse, got %v", err)
	}
}

func TestInitialLoadErrors(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=x")

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Open:  fs.open,
		Parse: parseConfig,
	})
	if r != nil {
		t.Fatalf("expected nil Reloader, got %v", r)
	}
	if !errors.Is(err, confighotreload.ErrParse) {
		t.Fatalf("expected ErrParse for port=x, got %v", err)
	}

	fs.set("config.txt", "port=0")
	r, err = confighotreload.New(confighotreload.Options[Config]{
		Path:     "config.txt",
		Open:     fs.open,
		Parse:    parseConfig,
		Validate: validateConfig,
	})
	if r != nil {
		t.Fatalf("expected nil Reloader, got %v", r)
	}
	if !errors.Is(err, confighotreload.ErrValidate) {
		t.Fatalf("expected ErrValidate for port=0, got %v", err)
	}
}

func TestReloadUnchanged(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	var parseCount atomic.Int64
	countingParse := func(data []byte) (Config, error) {
		parseCount.Add(1)
		return parseConfig(data)
	}

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:     "config.txt",
		Open:     fs.open,
		Parse:    countingParse,
		Validate: validateConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error in New: %v", err)
	}
	if parseCount.Load() != 1 {
		t.Fatalf("expected parseCount 1 after New, got %d", parseCount.Load())
	}

	changed, err := r.Reload()
	if err != nil {
		t.Fatalf("unexpected reload error: %v", err)
	}
	if changed {
		t.Fatalf("expected changed == false, got true")
	}
	if parseCount.Load() != 1 {
		t.Fatalf("expected parse call count to stay 1, got %d", parseCount.Load())
	}
}

func TestReloadSuccess(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	var onChangeCalls int
	var observedOld, observedNew Config

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:     "config.txt",
		Open:     fs.open,
		Parse:    parseConfig,
		Validate: validateConfig,
		OnChange: func(oldConfig, newConfig Config) {
			onChangeCalls++
			observedOld = oldConfig
			observedNew = newConfig
		},
	})
	if err != nil {
		t.Fatalf("unexpected error in New: %v", err)
	}

	fs.set("config.txt", "port=9090")
	changed, err := r.Reload()
	if err != nil {
		t.Fatalf("unexpected reload error: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed == true")
	}
	if r.Current().Port != 9090 {
		t.Fatalf("expected Current().Port == 9090, got %d", r.Current().Port)
	}
	if onChangeCalls != 1 {
		t.Fatalf("expected OnChange called once, got %d", onChangeCalls)
	}
	if observedOld.Port != 8080 || observedNew.Port != 9090 {
		t.Fatalf("unexpected OnChange configs: old=%+v, new=%+v", observedOld, observedNew)
	}
}

func TestReloadFailureAndRejectionDeduplication(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	var parseCount atomic.Int64
	countingParse := func(data []byte) (Config, error) {
		parseCount.Add(1)
		return parseConfig(data)
	}

	var onErrorCalls int
	var lastError error

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Open:  fs.open,
		Parse: countingParse,
		Validate: func(c Config) error {
			return validateConfig(c)
		},
		OnError: func(err error) {
			onErrorCalls++
			lastError = err
		},
	})
	if err != nil {
		t.Fatalf("unexpected error in New: %v", err)
	}

	fs.set("config.txt", "port=0")
	changed, err := r.Reload()
	if changed {
		t.Fatalf("expected changed == false")
	}
	if !errors.Is(err, confighotreload.ErrValidate) {
		t.Fatalf("expected ErrValidate, got %v", err)
	}
	if r.Current().Port != 8080 {
		t.Fatalf("expected Current().Port to stay 8080, got %d", r.Current().Port)
	}
	if onErrorCalls != 1 {
		t.Fatalf("expected OnError to be called once, got %d", onErrorCalls)
	}
	if !errors.Is(lastError, confighotreload.ErrValidate) {
		t.Fatalf("expected lastError to wrap ErrValidate, got %v", lastError)
	}

	countBeforeSecond := parseCount.Load()
	changed2, err2 := r.Reload()
	if changed2 {
		t.Fatalf("expected second reload changed == false")
	}
	if err2 != nil {
		t.Fatalf("expected second reload err == nil, got %v", err2)
	}
	if parseCount.Load() != countBeforeSecond {
		t.Fatalf("expected Parse not to be called on repeated rejected hash")
	}
	if onErrorCalls != 1 {
		t.Fatalf("expected OnError not to be called again, got %d", onErrorCalls)
	}

	// Revert to original content "port=8080"
	fs.set("config.txt", "port=8080")
	changed3, err3 := r.Reload()
	if err3 != nil {
		t.Fatalf("reverting error: %v", err3)
	}
	if changed3 {
		t.Fatalf("expected changed == false when reverting to already active hash")
	}
	if r.Current().Port != 8080 {
		t.Fatalf("expected Port 8080, got %d", r.Current().Port)
	}
}

func TestMaxBytesAndMissingFile(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	var lastError error
	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Open:  fs.open,
		Parse: parseConfig,
		OnError: func(err error) {
			lastError = err
		},
	})
	if err != nil {
		t.Fatalf("unexpected error in New: %v", err)
	}

	// Exactly 1048576 bytes
	// Leading zeros keep the content a valid "port=<digits>" for the strict test parser.
	padding := strings.Repeat("0", 1048576-len("port=8080"))
	fs.set("config.txt", "port="+padding+"8080")
	changed, err := r.Reload()
	if err != nil {
		t.Fatalf("expected 1048576 bytes accepted, got err: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed == true for new content")
	}

	// 1048577 bytes
	paddingOver := strings.Repeat("0", 1048577-len("port=8080"))
	fs.set("config.txt", "port="+paddingOver+"8080")
	changed, err = r.Reload()
	if changed {
		t.Fatalf("expected changed == false on oversized file")
	}
	if !errors.Is(err, confighotreload.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	if !errors.Is(lastError, confighotreload.ErrTooLarge) {
		t.Fatalf("expected lastError to match ErrTooLarge, got %v", lastError)
	}

	// Missing file
	fs.remove("config.txt")
	changed, err = r.Reload()
	if changed {
		t.Fatalf("expected changed == false on missing file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
	if r.Current().Port != 8080 {
		t.Fatalf("expected Port to remain 8080, got %d", r.Current().Port)
	}
}

func TestWatch(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	var onChangeCount atomic.Int64
	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Open:  fs.open,
		Parse: parseConfig,
		OnChange: func(oldConfig, newConfig Config) {
			onChangeCount.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())

	wDone := make(chan error, 1)
	go func() {
		wDone <- r.Watch(ctx, ticks)
	}()

	// First send with no changes
	ticks <- time.Now()

	// Change config
	fs.set("config.txt", "port=9090")
	ticks <- time.Now()

	// Check onChange called once
	for start := time.Now(); time.Since(start) < 2*time.Second; {
		if r.Current().Port == 9090 && onChangeCount.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if onChangeCount.Load() != 1 {
		t.Fatalf("expected 1 OnChange call, got %d", onChangeCount.Load())
	}

	cancel()
	select {
	case err := <-wDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Watch to terminate")
	}

	// Verify closed ticks returns nil
	ticksClosed := make(chan time.Time)
	close(ticksClosed)
	if err := r.Watch(context.Background(), ticksClosed); err != nil {
		t.Fatalf("expected nil error on closed ticks, got %v", err)
	}
}

func TestConcurrency(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Open:  fs.open,
		Parse: parseConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error in New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					p := r.Current().Port
					if p != 8080 && p != 9090 {
						t.Errorf("unexpected port observed: %d", p)
						return
					}
				}
			}
		}()
	}

	time.Sleep(20 * time.Millisecond)
	fs.set("config.txt", "port=9090")
	changed, err := r.Reload()
	if err != nil || !changed {
		t.Fatalf("expected successful swap: changed=%v, err=%v", changed, err)
	}

	wg.Wait()
	if r.Current().Port != 9090 {
		t.Fatalf("expected final port 9090, got %d", r.Current().Port)
	}
}

func TestDefaultOpenRealTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.env")
	if err := os.WriteFile(path, []byte("port=7070"), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  path,
		Parse: parseConfig,
	})
	if err != nil {
		t.Fatalf("unexpected error loading real file: %v", err)
	}
	if r.Current().Port != 7070 {
		t.Fatalf("expected port 7070, got %d", r.Current().Port)
	}

	if err := os.WriteFile(path, []byte("port=7071"), 0o600); err != nil {
		t.Fatalf("failed to update temp file: %v", err)
	}
	changed, err := r.Reload()
	if err != nil || !changed {
		t.Fatalf("expected successful reload: changed=%v, err=%v", changed, err)
	}
	if r.Current().Port != 7071 {
		t.Fatalf("expected port 7071, got %d", r.Current().Port)
	}
}

func TestEmptyFileHandling(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "")

	called := false
	parseEmpty := func(data []byte) (Config, error) {
		called = true
		if len(data) == 0 {
			return Config{Port: 80}, nil
		}
		return parseConfig(data)
	}

	r, err := confighotreload.New(confighotreload.Options[Config]{
		Path:  "config.txt",
		Open:  fs.open,
		Parse: parseEmpty,
	})
	if err != nil {
		t.Fatalf("unexpected error with empty file: %v", err)
	}
	if !called {
		t.Fatalf("expected Parse to be invoked for empty file")
	}
	if r.Current().Port != 80 {
		t.Fatalf("expected Port == 80, got %d", r.Current().Port)
	}
}

func TestCustomMaxBytes(t *testing.T) {
	fs := newMemFS()
	fs.set("config.txt", "port=8080")

	_, err := confighotreload.New(confighotreload.Options[Config]{
		Path:     "config.txt",
		Open:     fs.open,
		Parse:    parseConfig,
		MaxBytes: 5,
	})
	if !errors.Is(err, confighotreload.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge on custom max bytes, got %v", err)
	}
}

func TestReloaderCurrentNilPointer(t *testing.T) {
	var r confighotreload.Reloader[Config]
	cfg := r.Current()
	if cfg.Port != 0 {
		t.Fatalf("expected zero Config, got %+v", cfg)
	}
}
