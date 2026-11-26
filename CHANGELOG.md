# Changelog

All notable changes to this project are documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-26

### Added

- Structured errors with `slog.Attr` attributes, typed `Key[T]` lookups, stack traces, `%+v` formatting, `slog.LogValuer` support and panic recovery.
- Errors capture their stack into a fixed buffer, cache the formatted trace, and walk chains without allocating: logging an error allocates 4 times instead of 26.
- Benchmarks (`make bench`).
- A Makefile with the development tasks (`make help`).
- Requires Go 1.27 or later.
