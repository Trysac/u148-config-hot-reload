// Package confighotreload is a small, dependency-free Go library: Hot-reload a configuration file in Go: content-hash change detection, parse and validate before swap, atomic reads, change callbacks and an injected tick source
//
// The public API is every exported identifier of this root package. Functions are pure: they do
// no I/O, keep no global mutable state and report invalid input through returned errors.
package confighotreload
