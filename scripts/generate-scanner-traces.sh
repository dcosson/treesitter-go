#!/usr/bin/env bash
#
# generate-scanner-traces.sh
#
# Generates golden JSONL trace files for external scanner calls by:
# 1. Cloning tree-sitter at a pinned version
# 2. Applying scanner-trace.patch to instrument parser.c
# 3. Building the patched CLI with cargo
# 4. Running it against corpus test inputs for each language with an external scanner
# 5. Writing testdata/scanner-traces/{lang}.jsonl
#
# Prerequisites:
#   - Rust/cargo installed (for building tree-sitter CLI)
#   - C compiler (cc) for building grammar shared libraries
#   - build/grammars/ populated via `make fetch-test-grammars`
#
# Usage:
#   ./scripts/generate-scanner-traces.sh [--lang python] [--ts-version v0.25.3]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
GRAMMARS_DIR="$PROJECT_DIR/build/grammars"
TRACES_DIR="$PROJECT_DIR/testdata/scanner-traces"
PATCH_FILE="$SCRIPT_DIR/scanner-trace.patch"

# Tree-sitter version to clone — must be v0.26.0+ for --lib-path support.
TS_VERSION="v0.26.5"

# Languages with external scanners
SCANNER_LANGUAGES=(
  bash
  cpp
  css
  html
  javascript
  lua
  perl
  python
  ruby
  rust
  typescript
)

# Parse arguments
FILTER_LANG=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --lang)
      FILTER_LANG="$2"
      shift 2
      ;;
    --ts-version)
      TS_VERSION="$2"
      shift 2
      ;;
    --help|-h)
      echo "Usage: $0 [--lang LANG] [--ts-version VERSION]"
      echo "  --lang LANG        Only generate traces for this language"
      echo "  --ts-version VER   Tree-sitter version to clone (default: $TS_VERSION)"
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

# Detect shared library extension
case "$(uname -s)" in
  Darwin) DYLIB_EXT="dylib" ;;
  *)      DYLIB_EXT="so" ;;
esac

# Verify prerequisites
if ! command -v cargo >/dev/null 2>&1; then
  echo "Error: cargo not found. Install Rust: https://rustup.rs" >&2
  exit 1
fi

if ! command -v cc >/dev/null 2>&1; then
  echo "Error: C compiler (cc) not found." >&2
  exit 1
fi

if [ ! -d "$GRAMMARS_DIR" ]; then
  echo "Error: $GRAMMARS_DIR not found. Run 'make fetch-test-grammars' first." >&2
  exit 1
fi

if [ ! -f "$PATCH_FILE" ]; then
  echo "Error: $PATCH_FILE not found." >&2
  exit 1
fi

# Create temp directory for the patched tree-sitter build
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "=== Cloning tree-sitter $TS_VERSION ==="
git clone --depth=1 --branch "$TS_VERSION" \
  https://github.com/tree-sitter/tree-sitter.git \
  "$WORK_DIR/tree-sitter" 2>&1 | tail -2

echo "=== Applying scanner trace patch ==="
cd "$WORK_DIR/tree-sitter"

# Use the Python patcher which matches on code patterns rather than line numbers.
# This is robust across tree-sitter versions.
# Also patch build.rs to add -DTS_SCANNER_TRACE to the cc::Build config,
# ensuring the define is present regardless of CFLAGS caching.
python3 "$SCRIPT_DIR/apply-scanner-trace-patch.py" \
  "$WORK_DIR/tree-sitter/lib/src/parser.c" \
  --build-rs "$WORK_DIR/tree-sitter/lib/binding_rust/build.rs"

echo "=== Building patched tree-sitter CLI ==="
cargo build --release 2>&1 | tail -3

PATCHED_CLI="$WORK_DIR/tree-sitter/target/release/tree-sitter"
if [ ! -f "$PATCHED_CLI" ]; then
  echo "Error: patched CLI not built at $PATCHED_CLI" >&2
  exit 1
fi

echo "=== CLI built at $PATCHED_CLI ==="

# Create traces output directory
mkdir -p "$TRACES_DIR"

# Build the Go corpus extractor tool. This uses our internal/corpustest parser
# for exact byte-level consistency with the Go test harness, eliminating the
# previous Python dependency and its universal-newlines CRLF normalization bug.
echo "=== Building corpus extractor ==="
EXTRACTOR="$WORK_DIR/extract-corpus-inputs"
cd "$PROJECT_DIR"
go build -o "$EXTRACTOR" ./cmd/extract-corpus-inputs
echo "  Built $EXTRACTOR"

# Helper: get file extension for a language (for tree-sitter to auto-detect)
lang_extension() {
  case "$1" in
    bash)       echo "sh" ;;
    cpp)        echo "cpp" ;;
    css)        echo "css" ;;
    html)       echo "html" ;;
    javascript) echo "js" ;;
    lua)        echo "lua" ;;
    perl)       echo "pl" ;;
    python)     echo "py" ;;
    ruby)       echo "rb" ;;
    rust)       echo "rs" ;;
    typescript) echo "ts" ;;
    *)          echo "txt" ;;
  esac
}

# Process each language
for lang in "${SCANNER_LANGUAGES[@]}"; do
  if [ -n "$FILTER_LANG" ] && [ "$lang" != "$FILTER_LANG" ]; then
    continue
  fi

  grammar_dir="$GRAMMARS_DIR/tree-sitter-$lang"
  if [ ! -d "$grammar_dir" ]; then
    echo "Warning: grammar directory not found for $lang, skipping" >&2
    continue
  fi

  # TypeScript has a nested structure: the grammar is in typescript/ but
  # corpus tests may be at the top-level test/corpus.
  if [ "$lang" = "typescript" ]; then
    grammar_path="$grammar_dir/typescript"
    corpus_dirs=("$grammar_dir/test/corpus" "$grammar_dir/typescript/test/corpus" "$grammar_dir/common/test/corpus")
  else
    grammar_path="$grammar_dir"
    corpus_dirs=("$grammar_dir/test/corpus")
  fi

  # Check for scanner.c to confirm external scanner exists
  if [ ! -f "$grammar_path/src/scanner.c" ] && [ ! -f "$grammar_path/src/scanner.cc" ]; then
    echo "Warning: no scanner.c/scanner.cc found for $lang, skipping" >&2
    continue
  fi

  # Check for corpus test files
  has_corpus=false
  for corpus_dir in "${corpus_dirs[@]}"; do
    if [ -d "$corpus_dir" ]; then
      has_corpus=true
      break
    fi
  done
  if [ "$has_corpus" = false ]; then
    echo "Warning: no corpus directory found for $lang, skipping" >&2
    continue
  fi

  echo ""
  echo "=== Processing $lang ==="

  # Extract test inputs from corpus files using our Go tool.
  # The tool writes files with the correct extension directly, so no rename step needed.
  ext="$(lang_extension "$lang")"
  inputs_dir="$WORK_DIR/inputs/$lang"

  # Build args: pass all existing corpus dirs as positional arguments
  extractor_args=()
  for corpus_dir in "${corpus_dirs[@]}"; do
    if [ -d "$corpus_dir" ]; then
      extractor_args+=("$corpus_dir")
    fi
  done

  echo "  Extracting test inputs..."
  "$EXTRACTOR" --lang "$lang" --ext "$ext" --output-dir "$inputs_dir" "${extractor_args[@]}"

  # Count extracted inputs
  input_count=$(find "$inputs_dir" -name "*.$ext" 2>/dev/null | wc -l | tr -d ' ')
  if [ "$input_count" -eq 0 ]; then
    echo "Warning: no test inputs extracted for $lang, skipping" >&2
    continue
  fi

  # Build the grammar shared library directly with cc.
  # The tree-sitter CLI's internal grammar build hangs on large grammars
  # (perl: 479K lines, ruby: 471K lines), but raw cc compiles them in seconds.
  echo "  Building grammar library with cc..."
  dylib_path="$WORK_DIR/libs/${lang}.${DYLIB_EXT}"
  mkdir -p "$WORK_DIR/libs"

  src_files=("$grammar_path/src/parser.c")
  if [ -f "$grammar_path/src/scanner.c" ]; then
    src_files+=("$grammar_path/src/scanner.c")
  elif [ -f "$grammar_path/src/scanner.cc" ]; then
    src_files+=("$grammar_path/src/scanner.cc")
  fi

  cc -shared -fPIC -O2 -I "$grammar_path/src" "${src_files[@]}" -o "$dylib_path" 2>&1
  if [ ! -f "$dylib_path" ]; then
    echo "Warning: failed to compile grammar library for $lang, skipping" >&2
    continue
  fi
  echo "  Built $dylib_path"

  # Now parse all files using --lib-path to bypass CLI's internal grammar build
  trace_file="$TRACES_DIR/$lang.jsonl"
  > "$trace_file"  # truncate

  echo "  Generating traces..."
  file_count=0
  for input_file in "$inputs_dir"/*."$ext"; do
    [ -f "$input_file" ] || continue
    file_count=$((file_count + 1))

    # Extract the test name from the filename
    input_basename="$(basename "$input_file" ".$ext")"

    # Run the patched CLI; stderr has JSONL trace lines, stdout has parse output
    # We capture stderr and tag each line with the source file info
    trace_stderr="$WORK_DIR/trace_stderr.tmp"
    timeout 30 "$PATCHED_CLI" parse \
      --lib-path "$dylib_path" --lang-name "$lang" \
      "$input_file" --quiet 2>"$trace_stderr" || true

    # Read each trace line and add the file/lang metadata.
    # Only process lines starting with { (valid JSON); skip CLI warnings/errors.
    while IFS= read -r line; do
      case "$line" in
        \{*)
          rest="${line#\{}"
          echo "{\"lang\":\"$lang\",\"file\":\"$input_basename\",$rest" >> "$trace_file"
          ;;
      esac
    done < "$trace_stderr"
  done

  trace_count=$(wc -l < "$trace_file" | tr -d ' ')
  echo "  Generated $trace_count trace entries from $file_count files -> $trace_file"
done

echo ""
echo "=== Done ==="
echo "Trace files written to $TRACES_DIR/"
ls -lh "$TRACES_DIR"/*.jsonl 2>/dev/null || echo "(no trace files generated)"
