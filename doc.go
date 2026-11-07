// Package confighotreload provides robust, validated, lock-free hot-reloading
// for configuration files in Go services.
//
// Reloading avoids platform-dependent file watching by polling content hashes on
// ticks provided by the caller. Configurations are parsed and validated before
// atomic swaps, ensuring invalid updates are never applied.
package confighotreload
