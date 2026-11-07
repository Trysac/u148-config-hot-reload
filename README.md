# config-hot-reload

Hot-reload a configuration file in Go with content-hash change detection, pre-swap validation, lock-free reads, and injected tick sources. This library ensures that bad configurations never replace the current working configuration by enforcing validation before atomic swaps.

## The Problem

Services typically reload configuration on SIGHUP or via file watchers and swap in whatever they parsed. Consequently, a syntax error or a validation failure in a production configuration file can take the service down or lead to partially applied settings. Furthermore, filesystem watchers are often platform-specific and trigger multiple events for a single save operation.

## Installation

```bash
go get github.com/Trysac/u148-config-hot-reload
```

## Usage

```go
import "github.com/Trysac/u148-config-hot-reload"

type Config struct { Port int }

// ... inside your application code
opts := confighotreload.Options[Config]{
	Path: "app.conf",
	Parse: func(b []byte) (Config, error) { /* ... */ },
}
r, err := confighotreload.New(opts)
if err != nil {
	log.Fatal(err)
}
cfg := r.Current()
_ = r.Watch(ctx, time.NewTicker(5*time.Second).C)
```

## Public API

- `New[T any](opts Options[T]) (*Reloader[T], error)`: Initializes the reloader; the initial load must succeed.
- `Current() T`: Returns the latest validated configuration using a lock-free atomic pointer load.
- `Reload() (changed bool, err error)`: Triggers a manual reload cycle. Serialized by a mutex.
- `Watch(ctx context.Context, ticks <-chan time.Time) error`: Reloads on ticks provided by the caller.

### Errors & Limits

- Sentinel errors: `ErrParse`, `ErrValidate`, `ErrTooLarge`, `ErrInvalidOptions`. Wrapped with `fmt.Errorf`.
- `MaxBytes`: Defaults to 1 MiB. Reads exceeding this return `ErrTooLarge`.

## Why not Viper?

This library provides a validated, atomic swap that never applies bad config, has zero external dependencies, does not rely on platform-specific watchers, and uses a simplified API compared to larger configuration frameworks.

See [docs/SECURITY.md](docs/SECURITY.md) for vulnerability reporting and threat modeling.