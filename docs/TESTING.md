# Testing

## Running

```sh
go test -cover .          # tests with coverage summary
go run ./scripts/check    # every gate, as CI and the factory run it
```

## Policy

- Statement coverage of the library (every package except `scripts/`) must stay at or above 80 %.
- Tests live in package `confighotreload_test` and exercise the public API only.
- Prefer table-driven tests with `t.Run` subtests; document usage with runnable `Example` functions.
- The race detector is used to verify that 8 concurrent goroutines calling `Current()` while a reload swaps configuration only ever observe valid, non-corrupted states.
- Each bug fix starts with a failing test that reproduces it.
- Tests are deterministic: no network, no wall clock, no dependence on test order.

## Test Coverage

- `New`: Verifies `ErrParse` for malformed input and `ErrValidate` for logic errors.
- `Reload`: Tests content-hash deduplication (no parsing if hash matches), state retention on failure, and successful atomic swaps.
- `Watch`: Verifies reload frequency matches tick input, and context cancellation stops the loop.
- `Limits`: Confirms `ErrTooLarge` for files > 1 MiB and `os.ErrNotExist` behavior for missing files.
- `Concurrency`: Uses `t.Parallel()` and the race detector to ensure atomic consistency.