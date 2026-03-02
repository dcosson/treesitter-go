# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-03-02

Initial release. Pure-Go implementation of the tree-sitter parsing runtime.

### Added
- GLR parser with version forking, merging, and error recovery
- Incremental parsing with subtree reuse
- DFA-based lexer with keyword extraction
- External scanner support via Go interfaces
- 15 supported languages: Bash, C, C++, CSS, Go, HTML, Java, JavaScript, JSON, Python, Regex, Ruby, Rust, TSX, TypeScript
- 100% corpus test pass rate across all 15 languages (1619/1619 tests)
- Code generation pipeline (`tsgo-generate`) to convert C parse tables to Go
- Benchmark suite with Go-only and Go-vs-C comparison modes
- Differential testing against upstream C tree-sitter CLI
- Fuzz testing for all languages
- Scanner trace replay testing for external scanner parity

### Upstream
- Runtime logic based on tree-sitter **v0.26.6**
