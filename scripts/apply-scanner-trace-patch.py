#!/usr/bin/env python3
"""
Apply scanner trace instrumentation to tree-sitter's lib/src/parser.c.

This script modifies parser.c to emit JSONL trace lines to stderr on every
external scanner call when compiled with -DTS_SCANNER_TRACE.

It works by:
1. Adding the trace infrastructure code (base64 encoder, advance counter,
   JSONL emitter) after the existing includes
2. Wrapping the ts_parser__external_scanner_scan call site in ts_parser__lex
   with trace capture code

This is more robust than a line-number-based patch since it matches on
code patterns rather than exact line positions.
"""

import sys
import re


TRACE_INFRASTRUCTURE = r'''
// ==== Scanner Trace Instrumentation ====
//
// When TS_SCANNER_TRACE is defined, every external scanner call is logged
// to stderr as a JSONL line containing the scanner state before/after,
// valid_symbols, lookahead, byte offset, result, and advance count.

#ifdef TS_SCANNER_TRACE

#include <stdio.h>
#include <inttypes.h>

// Global counter for scanner calls within a parse
static _Thread_local uint64_t ts_trace_call_index = 0;

// Advance counter — we wrap the lexer's advance callback to count calls
static _Thread_local uint32_t ts_trace_advance_count = 0;
static _Thread_local void (*ts_trace_original_advance)(TSLexer *, bool) = NULL;

static void ts_trace_advance_wrapper(TSLexer *lexer, bool skip) {
  ts_trace_advance_count++;
  ts_trace_original_advance(lexer, skip);
}

// Base64 encoding for serialized scanner state
static const char ts_trace_b64_table[] =
  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

static void ts_trace_base64_encode(const char *data, unsigned len, char *out) {
  unsigned i = 0, j = 0;
  for (; i + 2 < len; i += 3) {
    unsigned char a = data[i], b = data[i+1], c = data[i+2];
    out[j++] = ts_trace_b64_table[a >> 2];
    out[j++] = ts_trace_b64_table[((a & 3) << 4) | (b >> 4)];
    out[j++] = ts_trace_b64_table[((b & 15) << 2) | (c >> 6)];
    out[j++] = ts_trace_b64_table[c & 63];
  }
  if (i < len) {
    unsigned char a = data[i];
    out[j++] = ts_trace_b64_table[a >> 2];
    if (i + 1 < len) {
      unsigned char b = data[i+1];
      out[j++] = ts_trace_b64_table[((a & 3) << 4) | (b >> 4)];
      out[j++] = ts_trace_b64_table[(b & 15) << 2];
    } else {
      out[j++] = ts_trace_b64_table[(a & 3) << 4];
      out[j++] = '=';
    }
    out[j++] = '=';
  }
  out[j] = '\0';
}

// Emit a JSONL trace line to stderr
static void ts_trace_emit(
  TSParser *self,
  unsigned external_lex_state,
  uint32_t byte_offset,
  int32_t lookahead,
  const bool *valid_symbols,
  const char *pre_state_buf,
  unsigned pre_state_len,
  bool matched,
  uint32_t advances,
  const char *post_state_buf,
  unsigned post_state_len
) {
  // Base64 encode pre and post states
  char pre_b64[1400], post_b64[1400];
  ts_trace_base64_encode(pre_state_buf, pre_state_len, pre_b64);
  ts_trace_base64_encode(post_state_buf, post_state_len, post_b64);

  uint32_t ext_token_count = self->language->external_token_count;
  TSSymbol result_symbol = self->lexer.data.result_symbol;
  uint32_t token_end_byte = self->lexer.token_end_position.bytes;

  // Build valid_symbols JSON array
  char vs_buf[512];
  int vs_pos = 0;
  vs_buf[vs_pos++] = '[';
  for (uint32_t i = 0; i < ext_token_count && vs_pos < 500; i++) {
    if (i > 0) vs_buf[vs_pos++] = ',';
    vs_buf[vs_pos++] = (valid_symbols && valid_symbols[i]) ? '1' : '0';
  }
  vs_buf[vs_pos++] = ']';
  vs_buf[vs_pos] = '\0';

  fprintf(stderr,
    "{\"call_index\":%" PRIu64
    ",\"input\":{\"byte_offset\":%" PRIu32
    ",\"lookahead\":%" PRId32
    ",\"valid_symbols\":%s"
    ",\"scanner_state_before\":\"%s\"}"
    ",\"output\":{\"matched\":%s"
    ",\"result_symbol\":%" PRIu16
    ",\"token_end_byte\":%" PRIu32
    ",\"advances\":%" PRIu32
    ",\"scanner_state_after\":\"%s\"}}\n",
    ts_trace_call_index++,
    byte_offset,
    lookahead,
    vs_buf,
    pre_b64,
    matched ? "true" : "false",
    result_symbol,
    token_end_byte,
    advances,
    post_b64
  );
}

#endif // TS_SCANNER_TRACE

'''

# The code that wraps the scan call site
SCAN_CALL_REPLACEMENT = r'''
#ifdef TS_SCANNER_TRACE
      // Capture pre-scan state
      char trace_pre_state[TREE_SITTER_SERIALIZATION_BUFFER_SIZE];
      unsigned trace_pre_len = self->language->external_scanner.serialize(
        self->external_scanner_payload, trace_pre_state
      );
      uint32_t trace_byte_offset = self->lexer.current_position.bytes;
      int32_t trace_lookahead = self->lexer.data.lookahead;
      const bool *trace_valid_symbols = ts_language_enabled_external_tokens(
        self->language, lex_mode.external_lex_state
      );

      // Install advance counter wrapper
      ts_trace_advance_count = 0;
      ts_trace_original_advance = self->lexer.data.advance;
      self->lexer.data.advance = ts_trace_advance_wrapper;

      found_token = ts_parser__external_scanner_scan(self, lex_mode.external_lex_state);

      // Restore original advance
      self->lexer.data.advance = ts_trace_original_advance;

      // Capture post-scan state
      char trace_post_state[TREE_SITTER_SERIALIZATION_BUFFER_SIZE];
      unsigned trace_post_len = self->language->external_scanner.serialize(
        self->external_scanner_payload, trace_post_state
      );

      ts_trace_emit(self, lex_mode.external_lex_state,
        trace_byte_offset, trace_lookahead, trace_valid_symbols,
        trace_pre_state, trace_pre_len,
        found_token, ts_trace_advance_count,
        trace_post_state, trace_post_len);
#else
      found_token = ts_parser__external_scanner_scan(self, lex_mode.external_lex_state);
#endif
'''


def apply_patch(parser_c_path):
    with open(parser_c_path, 'r') as f:
        content = f.read()

    # 1. Insert trace infrastructure after the first function definition block
    #    We look for the ts_parser__external_scanner_create function and insert before it
    marker = 'static void ts_parser__external_scanner_create('
    idx = content.find(marker)
    if idx == -1:
        print("Error: could not find ts_parser__external_scanner_create", file=sys.stderr)
        sys.exit(1)

    content = content[:idx] + TRACE_INFRASTRUCTURE + content[idx:]

    # 2. Replace the scan call site in ts_parser__lex
    #    Look for the pattern:
    #      ts_parser__external_scanner_deserialize(self, external_token);
    #      found_token = ts_parser__external_scanner_scan(self, lex_mode.external_lex_state);
    scan_pattern = re.compile(
        r'(ts_parser__external_scanner_deserialize\(self,\s*external_token\);\s*\n)'
        r'(\s*)(found_token\s*=\s*ts_parser__external_scanner_scan\(self,\s*lex_mode\.external_lex_state\);)',
        re.MULTILINE
    )

    match = scan_pattern.search(content)
    if not match:
        print("Error: could not find scan call site pattern", file=sys.stderr)
        sys.exit(1)

    # Replace only the found_token = ... line with the #ifdef wrapper
    replacement = match.group(1) + SCAN_CALL_REPLACEMENT
    content = content[:match.start()] + replacement + content[match.end():]

    with open(parser_c_path, 'w') as f:
        f.write(content)

    print(f"Successfully patched {parser_c_path}")


def patch_build_rs(build_rs_path):
    """Add -DTS_SCANNER_TRACE to the cc::Build configuration in build.rs."""
    with open(build_rs_path, 'r') as f:
        content = f.read()

    # Insert .define("TS_SCANNER_TRACE", None) before .warnings(false)
    marker = '.warnings(false)'
    if marker not in content:
        print(f"Warning: could not find '{marker}' in build.rs, skipping build.rs patch", file=sys.stderr)
        return

    content = content.replace(marker, '.define("TS_SCANNER_TRACE", None)\n        ' + marker)

    with open(build_rs_path, 'w') as f:
        f.write(content)

    print(f"Successfully patched {build_rs_path}")


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <path/to/parser.c> [--build-rs path/to/build.rs]", file=sys.stderr)
        sys.exit(1)
    apply_patch(sys.argv[1])

    # Also patch build.rs if --build-rs is provided
    if len(sys.argv) >= 4 and sys.argv[2] == '--build-rs':
        patch_build_rs(sys.argv[3])
