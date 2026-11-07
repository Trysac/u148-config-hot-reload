# Architecture

## Overview

`confighotreload` enables safe hot-reloading of configuration files in Go applications.

### Invalidation and Safety Flow

1. **Read with bounds:** Reads the configuration using `io.LimitReader(r, maxBytes+1)`. If bytes exceed `maxBytes`, `ErrTooLarge` is returned without allocating unbounded memory.
2. **Hash Check:** Computes `sha256` of file content.
   - If the hash matches `currentHash`, reload is a no-op returning `(false, nil)`.
   - If the hash matches `rejectedHash`, reload is a no-op returning `(false, nil)` without re-invoking `Parse` or `OnError`.
3. **Parse and Validate:** Calls caller-supplied `Parse` and optional `Validate`.
   - If parse fails: returns wrapped `ErrParse`, updates `rejectedHash`, retains existing configuration, and invokes `OnError`.
   - If validation fails: returns wrapped `ErrValidate`, updates `rejectedHash`, retains existing configuration, and invokes `OnError`.
4. **Atomic Pointer Swap:** On success, replaces the pointer in `atomic.Pointer[T]`, updates `currentHash`, clears rejection tracking, and invokes `OnChange` outside critical serialization if needed.

### Concurrency Model

- Reads via `Current()` are lock-free and race-free using `sync/atomic.Pointer`.
- Reloads are serialized with a `sync.Mutex`.
