# Security notes

## Threat model

`github.com/Trysac/u148-config-hot-reload` is a pure library without I/O side effects. 

### Defended

- **Malformed configurations**: Callbacks for `Parse` and `Validate` ensure that invalid data is caught before reaching the application state.
- **Unbounded Memory**: `MaxBytes` prevents reading excessively large files into memory (default limit 1 MiB).
- **Race conditions**: `Current()` uses `sync/atomic.Pointer` for lock-free read access, ensuring consistent reads even during reloads.

### Not defended

- **Panic propagation**: Callbacks (`OnChange`, `OnError`) run on the reloading goroutine; a panic within a callback is not recovered by the library.
- **Input Source**: The library relies on the provided `Open` function. If `Open` is manipulated, the library cannot verify the file's provenance.

## Reporting

To report a vulnerability, please see the contact details provided in the repository guidelines.