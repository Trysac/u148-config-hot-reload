package confighotreload

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxBytes is the default maximum file size (1 MiB) processed during load and reload.
const DefaultMaxBytes int64 = 1048576 // 1 MiB

// Sentinel errors returned by the package.
var (
	ErrParse          = errors.New("confighotreload: parse error")
	ErrValidate       = errors.New("confighotreload: validation error")
	ErrTooLarge       = errors.New("confighotreload: configuration file too large")
	ErrInvalidOptions = errors.New("confighotreload: invalid options")
)

// Options configures a Reloader instance.
type Options[T any] struct {
	// Path to the configuration file.
	Path string

	// Open specifies the function used to open the file at Path. Defaults to os.Open.
	Open func(name string) (io.ReadCloser, error)

	// Parse converts raw bytes into configuration type T. Required.
	Parse func(data []byte) (T, error)

	// Validate optionally validates the parsed configuration. Returns an error if invalid.
	Validate func(T) error

	// OnChange is invoked on successful reloads with the previous and new configurations.
	OnChange func(oldConfig, newConfig T)

	// OnError is invoked when an error occurs during a reload.
	OnError func(err error)

	// MaxBytes specifies the maximum accepted file size. Defaults to 1 MiB if <= 0.
	MaxBytes int64
}

// Reloader safely reloads and provides atomic access to a validated configuration.
type Reloader[T any] struct {
	opts         Options[T]
	maxBytes     int64
	current      atomic.Pointer[T]
	mu           sync.Mutex
	currentHash  [32]byte
	rejectedHash [32]byte
	hasRejected  bool
}

// New creates a new Reloader and performs the initial configuration load.
// The initial load must succeed; otherwise a non-nil error is returned.
func New[T any](opts Options[T]) (*Reloader[T], error) {
	if opts.Path == "" || opts.Parse == nil {
		return nil, fmt.Errorf("%w: Path and Parse are required", ErrInvalidOptions)
	}

	if opts.Open == nil {
		opts.Open = func(name string) (io.ReadCloser, error) {
			return os.Open(name)
		}
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	r := &Reloader[T]{
		opts:     opts,
		maxBytes: maxBytes,
	}

	content, hash, err := r.readFile()
	if err != nil {
		return nil, err
	}

	cfg, err := opts.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParse, err)
	}

	if opts.Validate != nil {
		if err := opts.Validate(cfg); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrValidate, err)
		}
	}

	r.current.Store(&cfg)
	r.currentHash = hash
	return r, nil
}

// Current returns the latest successfully loaded configuration lock-free.
func (r *Reloader[T]) Current() T {
	p := r.current.Load()
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// Reload attempts to re-read, parse, validate, and atomically swap the configuration.
// If the file hash matches the current hash or the last rejected hash, Reload returns (false, nil)
// without invoking Parse or OnError.
func (r *Reloader[T]) Reload() (changed bool, err error) {
	r.mu.Lock()

	content, hash, readErr := r.readFile()
	if readErr != nil {
		r.mu.Unlock()
		if r.opts.OnError != nil {
			r.opts.OnError(readErr)
		}
		return false, readErr
	}

	if hash == r.currentHash {
		r.mu.Unlock()
		return false, nil
	}
	if r.hasRejected && hash == r.rejectedHash {
		r.mu.Unlock()
		return false, nil
	}

	newCfg, parseErr := r.opts.Parse(content)
	if parseErr != nil {
		err = fmt.Errorf("%w: %v", ErrParse, parseErr)
		r.rejectedHash = hash
		r.hasRejected = true
		r.mu.Unlock()
		if r.opts.OnError != nil {
			r.opts.OnError(err)
		}
		return false, err
	}

	if r.opts.Validate != nil {
		if valErr := r.opts.Validate(newCfg); valErr != nil {
			err = fmt.Errorf("%w: %v", ErrValidate, valErr)
			r.rejectedHash = hash
			r.hasRejected = true
			r.mu.Unlock()
			if r.opts.OnError != nil {
				r.opts.OnError(err)
			}
			return false, err
		}
	}

	oldPtr := r.current.Load()
	var oldCfg T
	if oldPtr != nil {
		oldCfg = *oldPtr
	}

	r.current.Store(&newCfg)
	r.currentHash = hash
	r.hasRejected = false
	r.mu.Unlock()

	if r.opts.OnChange != nil {
		r.opts.OnChange(oldCfg, newCfg)
	}

	return true, nil
}

// Watch polls Reload on each signal from ticks until ctx is cancelled or ticks is closed.
// If ctx is cancelled, Watch returns ctx.Err(). If ticks is closed, Watch returns nil.
func (r *Reloader[T]) Watch(ctx context.Context, ticks <-chan time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return nil
			}
			_, _ = r.Reload()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func (r *Reloader[T]) readFile() ([]byte, [32]byte, error) {
	rc, err := r.opts.Open(r.opts.Path)
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("open %q: %w", r.opts.Path, err)
	}
	defer rc.Close()

	limit := r.maxBytes + 1
	buf, err := io.ReadAll(io.LimitReader(rc, limit))
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("read %q: %w", r.opts.Path, err)
	}

	if int64(len(buf)) > r.maxBytes {
		return nil, [32]byte{}, fmt.Errorf("%w: limit of %d bytes exceeded", ErrTooLarge, r.maxBytes)
	}

	hash := sha256.Sum256(buf)
	return buf, hash, nil
}
