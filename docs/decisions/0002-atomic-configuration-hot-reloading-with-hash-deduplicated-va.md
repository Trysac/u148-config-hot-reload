# Atomic configuration hot-reloading with hash-deduplicated validation

Status: accepted (generated, pending human review)

## Context

Services reloading configuration files directly or via OS-specific watchers (such as fsnotify) often encounter broken configurations, partial writes, multiple duplicate events, or platform-dependent watcher behaviors. Swapping unvalidated configurations into running services can cause downtime.

## Decision

Implement confighotreload using pure standard library primitives. Readers access the active configuration via atomic.Pointer without acquiring locks. Reloads are serialized, capped to a maximum byte length (default 1 MiB), hashed via SHA-256 to deduplicate unchanged files and repeating rejected files, and parsed/validated prior to swapping. Reload triggers are supplied via channels (Watch) or manual invocations (Reload).

## Consequences

Hot reloading requires no external dependencies or OS filesystem notifications. Broken files never replace good configurations, and repeated reloads against invalid files do not flood error callbacks. SIGHUP and timer-based polling can be integrated seamlessly.
