package generate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExtractGrammar parses a tree-sitter parser.c file and extracts all tables
// and metadata into a Grammar intermediate representation.
func ExtractGrammar(parserC string) (*Grammar, error) {
	g := &Grammar{}

	// Extract grammar name from the language function.
	if err := extractName(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting name: %w", err)
	}

	// Extract #define constants.
	if err := extractConstants(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting constants: %w", err)
	}

	// Build enum maps for resolving C identifier names to integer indices.
	g.symbolEnum = extractEnum(parserC, "ts_symbol_identifiers")
	// Add builtins that aren't in the enum.
	g.symbolEnum["ts_builtin_sym_end"] = 0
	g.fieldEnum = extractFieldEnum(parserC)

	// Extract symbol names.
	if err := extractSymbolNames(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting symbol names: %w", err)
	}

	// Extract symbol metadata.
	if err := extractSymbolMetadata(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting symbol metadata: %w", err)
	}

	// Extract field names.
	extractFieldNames(g, parserC)

	// Extract field map slices and entries.
	extractFieldMapSlices(g, parserC)
	extractFieldMapEntries(g, parserC)

	// Extract alias sequences.
	extractAliasSequences(g, parserC)

	// Extract non-terminal alias map.
	extractNonTerminalAliasMap(g, parserC)

	// Extract primary state IDs.
	if err := extractPrimaryStateIDs(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting primary state IDs: %w", err)
	}

	// Extract parse actions.
	if err := extractParseActions(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting parse actions: %w", err)
	}

	// Extract large parse table.
	if err := extractParseTable(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting parse table: %w", err)
	}

	// Extract small parse table.
	extractSmallParseTable(g, parserC)
	extractSmallParseTableMap(g, parserC)

	// Extract lex modes.
	if err := extractLexModes(g, parserC); err != nil {
		return nil, fmt.Errorf("extracting lex modes: %w", err)
	}

	// Extract lex function DFA.
	if err := extractLexFunction(g, parserC, false); err != nil {
		return nil, fmt.Errorf("extracting lex function: %w", err)
	}

	// Extract keyword lex function if present.
	extractLexFunction(g, parserC, true)

	// Extract public symbol map.
	extractPublicSymbolMap(g, parserC)

	// Extract external scanner data.
	extractExternalScannerSymbolMap(g, parserC)
	extractExternalScannerStates(g, parserC)

	return g, nil
}

// extractName finds the grammar name from tree_sitter_<name>().
func extractName(g *Grammar, src string) error {
	// Match the language function: const TSLanguage *tree_sitter_<name>(void)
	// This avoids matching external scanner functions like tree_sitter_<name>_external_scanner_create.
	re := regexp.MustCompile(`TSLanguage\s*\*\s*tree_sitter_(\w+)\s*\(void\)`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return fmt.Errorf("could not find tree_sitter_<name>(void) function")
	}
	g.Name = m[1]
	return nil
}

// extractConstants extracts #define constants.
func extractConstants(g *Grammar, src string) error {
	defs := map[string]*int{
		"SYMBOL_COUNT":             &g.SymbolCount,
		"ALIAS_COUNT":             &g.AliasCount,
		"TOKEN_COUNT":             &g.TokenCount,
		"EXTERNAL_TOKEN_COUNT":    &g.ExternalTokenCount,
		"STATE_COUNT":             &g.StateCount,
		"LARGE_STATE_COUNT":       &g.LargeStateCount,
		"PRODUCTION_ID_COUNT":     &g.ProductionIDCount,
		"FIELD_COUNT":             &g.FieldCount,
		"MAX_ALIAS_SEQUENCE_LENGTH": &g.MaxAliasSequenceLength,
	}

	re := regexp.MustCompile(`#define\s+(\w+)\s+(\d+)`)
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		if ptr, ok := defs[m[1]]; ok {
			v, err := strconv.Atoi(m[2])
			if err != nil {
				return fmt.Errorf("parsing %s: %w", m[1], err)
			}
			*ptr = v
		}
	}

	if g.SymbolCount == 0 {
		return fmt.Errorf("SYMBOL_COUNT not found")
	}
	return nil
}

// extractSymbolNames extracts the ts_symbol_names[] array.
func extractSymbolNames(g *Grammar, src string) error {
	block := extractArrayBlock(src, "ts_symbol_names")
	if block == "" {
		return fmt.Errorf("ts_symbol_names not found")
	}

	// Match C string assignments: [name] = "value"
	// Must handle escaped quotes in values like "\""
	re := regexp.MustCompile(`\[(\w+)\]\s*=\s*"((?:[^"\\]|\\.)*)"`)
	names := make([]string, g.SymbolCount)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx := g.resolveSymbolIndex(m[1])
		if idx >= 0 && idx < g.SymbolCount {
			names[idx] = unescapeCString(m[2])
		}
	}
	g.SymbolNames = names
	return nil
}

// extractSymbolMetadata extracts ts_symbol_metadata[].
func extractSymbolMetadata(g *Grammar, src string) error {
	block := extractArrayBlock(src, "ts_symbol_metadata")
	if block == "" {
		return fmt.Errorf("ts_symbol_metadata not found")
	}

	meta := make([]SymMeta, g.SymbolCount)

	// The metadata entries span multiple lines:
	//   [sym_name] = {
	//     .visible = true,
	//     .named = true,
	//   },
	// We split on "[" to find each entry.
	entries := strings.Split(block, "[")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// Extract the symbol name before "]".
		closeBracket := strings.Index(entry, "]")
		if closeBracket < 0 {
			continue
		}
		symName := strings.TrimSpace(entry[:closeBracket])
		idx := g.resolveSymbolIndex(symName)
		if idx < 0 || idx >= g.SymbolCount {
			continue
		}

		// The rest contains the metadata fields.
		rest := entry[closeBracket:]
		meta[idx].Visible = strings.Contains(rest, ".visible = true")
		meta[idx].Named = strings.Contains(rest, ".named = true")
		meta[idx].Supertype = strings.Contains(rest, ".supertype = true")
	}
	g.SymbolMetadata = meta
	return nil
}

// extractFieldNames extracts ts_field_names[].
func extractFieldNames(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_field_names")
	if block == "" {
		g.FieldNames = []string{""}
		return
	}

	names := make([]string, g.FieldCount+1)
	// Match entries like: [field_key] = "key", or [0] = NULL, or [1] = "key"
	re := regexp.MustCompile(`\[(\w+)\]\s*=\s*"([^"]*)"`)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx := g.resolveFieldID(m[1])
		if int(idx) < len(names) {
			names[idx] = m[2]
		}
	}
	g.FieldNames = names
}

// extractFieldMapSlices extracts ts_field_map_slices[].
func extractFieldMapSlices(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_field_map_slices")
	if block == "" {
		return
	}

	re := regexp.MustCompile(`\[(\d+)\]\s*=\s*\{\.index\s*=\s*(\d+),\s*\.length\s*=\s*(\d+)\}`)
	slices := make([]FieldSlice, g.ProductionIDCount)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx, _ := strconv.Atoi(m[1])
		index, _ := strconv.Atoi(m[2])
		length, _ := strconv.Atoi(m[3])
		if idx < len(slices) {
			slices[idx] = FieldSlice{Index: uint16(index), Length: uint16(length)}
		}
	}
	g.FieldMapSlices = slices
}

// extractFieldMapEntries extracts ts_field_map_entries[].
func extractFieldMapEntries(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_field_map_entries")
	if block == "" {
		return
	}

	re := regexp.MustCompile(`\{(\w+),\s*(\d+)(?:,\s*\.inherited\s*=\s*true)?\}`)
	var entries []FieldEntry
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		fieldID := g.resolveFieldID(m[1])
		childIdx, _ := strconv.Atoi(m[2])
		inherited := strings.Contains(m[0], ".inherited = true")
		entries = append(entries, FieldEntry{
			FieldID:    fieldID,
			ChildIndex: uint16(childIdx),
			Inherited:  inherited,
		})
	}
	g.FieldMapEntries = entries
}

// extractAliasSequences extracts ts_alias_sequences[][].
func extractAliasSequences(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_alias_sequences")
	if block == "" {
		return
	}

	size := g.ProductionIDCount * g.MaxAliasSequenceLength
	seqs := make([]uint16, size)

	// Parse [prodID][childIdx] = symbol entries.
	re := regexp.MustCompile(`\[(\d+)\]\[(\d+)\]\s*=\s*(\w+)`)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		prodID, _ := strconv.Atoi(m[1])
		childIdx, _ := strconv.Atoi(m[2])
		sym := g.resolveSymbolValue(m[3])
		idx := prodID*g.MaxAliasSequenceLength + childIdx
		if idx < len(seqs) {
			seqs[idx] = sym
		}
	}
	g.AliasSequences = seqs
}

// extractNonTerminalAliasMap extracts ts_non_terminal_alias_map[].
func extractNonTerminalAliasMap(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_non_terminal_alias_map")
	if block == "" {
		return
	}

	re := regexp.MustCompile(`(\d+)`)
	var values []uint16
	for _, m := range re.FindAllString(block, -1) {
		v, _ := strconv.Atoi(m)
		values = append(values, uint16(v))
	}
	g.NonTerminalAliasMap = values
}

// extractPrimaryStateIDs extracts ts_primary_state_ids[].
func extractPrimaryStateIDs(g *Grammar, src string) error {
	block := extractArrayBlock(src, "ts_primary_state_ids")
	if block == "" {
		return fmt.Errorf("ts_primary_state_ids not found")
	}

	re := regexp.MustCompile(`\[(\d+)\]\s*=\s*(\d+)`)
	ids := make([]uint16, g.StateCount)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx, _ := strconv.Atoi(m[1])
		val, _ := strconv.Atoi(m[2])
		if idx < len(ids) {
			ids[idx] = uint16(val)
		}
	}
	g.PrimaryStateIDs = ids
	return nil
}

// extractParseActions extracts ts_parse_actions[].
//
// The C format has each line as: [idx] = {header}, ACTION(), ACTION(), ...
// where idx is the starting position in the flat array. The header occupies
// position idx, followed by count action entries at idx+1, idx+2, etc.
func extractParseActions(g *Grammar, src string) error {
	block := extractArrayBlock(src, "ts_parse_actions")
	if block == "" {
		return fmt.Errorf("ts_parse_actions not found")
	}

	// First pass: find the maximum index to size the array.
	indexRe := regexp.MustCompile(`\[(\d+)\]\s*=`)
	maxIdx := 0
	for _, m := range indexRe.FindAllStringSubmatch(block, -1) {
		idx, _ := strconv.Atoi(m[1])
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	// Parse each line.
	type indexedLine struct {
		idx  int
		line string
	}
	var lines []indexedLine
	lineRe := regexp.MustCompile(`(?m)^\s*\[(\d+)\]\s*=\s*(.+)$`)
	for _, m := range lineRe.FindAllStringSubmatch(block, -1) {
		idx, _ := strconv.Atoi(m[1])
		lines = append(lines, indexedLine{idx: idx, line: m[2]})
	}

	// Determine total entries needed: for each line, header + count actions.
	// We need to know the count from the header to determine how many slots.
	// Use a map first, then flatten.
	entries := make(map[int][]ActionEntry)
	for _, il := range lines {
		line := il.line
		var group []ActionEntry

		// Parse header: {.entry = {.count = N, .reusable = true/false}}
		headerRe := regexp.MustCompile(`\{\.entry\s*=\s*\{\.count\s*=\s*(\d+),\s*\.reusable\s*=\s*(\w+)\}\}`)
		hm := headerRe.FindStringSubmatch(line)
		if hm == nil {
			continue
		}
		count, _ := strconv.Atoi(hm[1])
		reusable := hm[2] == "true"
		group = append(group, ActionEntry{
			IsHeader: true,
			Count:    count,
			Reusable: reusable,
		})

		// Parse action macros after the header.
		rest := line[headerRe.FindStringIndex(line)[1]:]
		group = append(group, g.parseActionMacros(rest)...)

		entries[il.idx] = group
	}

	// Flatten into a contiguous array.
	// Find total size: max index + max group length.
	totalSize := 0
	for idx, group := range entries {
		end := idx + len(group)
		if end > totalSize {
			totalSize = end
		}
	}

	actions := make([]ActionEntry, totalSize)
	for idx, group := range entries {
		for i, entry := range group {
			actions[idx+i] = entry
		}
	}

	g.ParseActions = actions
	return nil
}

// parseActionMacros parses action macros from a line fragment.
func (g *Grammar) parseActionMacros(line string) []ActionEntry {
	var actions []ActionEntry

	// RECOVER()
	if strings.Contains(line, "RECOVER()") {
		actions = append(actions, ActionEntry{ActionType: "recover"})
	}

	// SHIFT_EXTRA()
	shiftExtraRe := regexp.MustCompile(`(?:^|,)\s*SHIFT_EXTRA\(\)`)
	for range shiftExtraRe.FindAllString(line, -1) {
		actions = append(actions, ActionEntry{ActionType: "shift", ShiftExtra: true})
	}

	// SHIFT_REPEAT(N)
	shiftRepeatRe := regexp.MustCompile(`SHIFT_REPEAT\((\d+)\)`)
	for _, m := range shiftRepeatRe.FindAllStringSubmatch(line, -1) {
		state, _ := strconv.Atoi(m[1])
		actions = append(actions, ActionEntry{
			ActionType:      "shift",
			ShiftState:      uint16(state),
			ShiftRepetition: true,
		})
	}

	// SHIFT(N) - must not match SHIFT_EXTRA or SHIFT_REPEAT
	shiftRe := regexp.MustCompile(`(?:^|[,\s])SHIFT\((\d+)\)`)
	for _, m := range shiftRe.FindAllStringSubmatch(line, -1) {
		state, _ := strconv.Atoi(m[1])
		actions = append(actions, ActionEntry{
			ActionType: "shift",
			ShiftState: uint16(state),
		})
	}

	// REDUCE(sym, childCount, dynPrec, prodID)
	reduceRe := regexp.MustCompile(`REDUCE\((\w+),\s*(\d+),\s*(-?\d+),\s*(\d+)\)`)
	for _, m := range reduceRe.FindAllStringSubmatch(line, -1) {
		sym := g.resolveSymbolValue(m[1])
		childCount, _ := strconv.Atoi(m[2])
		dynPrec, _ := strconv.Atoi(m[3])
		prodID, _ := strconv.Atoi(m[4])
		actions = append(actions, ActionEntry{
			ActionType:       "reduce",
			ReduceSymbol:     sym,
			ReduceChildCount: childCount,
			ReduceDynPrec:    dynPrec,
			ReduceProdID:     uint16(prodID),
		})
	}

	// ACCEPT_INPUT()
	if strings.Contains(line, "ACCEPT_INPUT()") {
		actions = append(actions, ActionEntry{ActionType: "accept"})
	}

	return actions
}

// extractParseTable extracts the large state parse table.
func extractParseTable(g *Grammar, src string) error {
	block := extractArrayBlock(src, "ts_parse_table")
	if block == "" {
		return fmt.Errorf("ts_parse_table not found")
	}

	table := make([]uint16, g.LargeStateCount*g.SymbolCount)

	// Parse each state row.
	stateRe := regexp.MustCompile(`\[(\d+)\]\s*=\s*\{([^}]+)\}`)
	for _, m := range stateRe.FindAllStringSubmatch(block, -1) {
		stateIdx, _ := strconv.Atoi(m[1])
		body := m[2]

		// Parse entries: [symbol] = value
		entryRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*(\w+)\(([^)]*)\)`)
		for _, em := range entryRe.FindAllStringSubmatch(body, -1) {
			symName := em[1]
			macro := em[2]
			args := em[3]

			symIdx := g.resolveSymbolIndex(symName)
			if symIdx < 0 || symIdx >= g.SymbolCount {
				continue
			}

			var value uint16
			switch macro {
			case "ACTIONS":
				v, _ := strconv.Atoi(strings.TrimSpace(args))
				value = uint16(v)
			case "STATE":
				v, _ := strconv.Atoi(strings.TrimSpace(args))
				value = uint16(v)
			}

			idx := stateIdx*g.SymbolCount + symIdx
			if idx < len(table) {
				table[idx] = value
			}
		}
	}

	g.ParseTable = table
	return nil
}

// extractSmallParseTable extracts ts_small_parse_table[].
func extractSmallParseTable(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_small_parse_table")
	if block == "" {
		return
	}

	var values []uint16
	// The small parse table is a flat array of uint16 values.
	// Parse all numeric values, macro references, and symbol enum names.
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}

		// Handle [offset] = N, format (these are section markers).
		if strings.HasPrefix(line, "[") && strings.Contains(line, "] =") {
			re := regexp.MustCompile(`\]\s*=\s*(\d+)`)
			m := re.FindStringSubmatch(line)
			if m != nil {
				v, _ := strconv.Atoi(m[1])
				values = append(values, uint16(v))
			}
			continue
		}

		// Parse ACTIONS(N), STATE(N), bare numbers, and symbol names.
		tokens := tokenizeLine(line)
		for _, tok := range tokens {
			if v, ok := g.parseTableToken(tok); ok {
				values = append(values, v)
			}
		}
	}

	g.SmallParseTable = values
}

// extractSmallParseTableMap extracts ts_small_parse_table_map[].
func extractSmallParseTableMap(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_small_parse_table_map")
	if block == "" {
		return
	}

	re := regexp.MustCompile(`\[SMALL_STATE\((\d+)\)\]\s*=\s*(\d+)`)
	// Count small states.
	numSmall := g.StateCount - g.LargeStateCount
	mapping := make([]uint32, numSmall)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		stateID, _ := strconv.Atoi(m[1])
		offset, _ := strconv.Atoi(m[2])
		idx := stateID - g.LargeStateCount
		if idx >= 0 && idx < numSmall {
			mapping[idx] = uint32(offset)
		}
	}
	g.SmallParseTableMap = mapping
}

// extractLexModes extracts ts_lex_modes[].
func extractLexModes(g *Grammar, src string) error {
	block := extractArrayBlock(src, "ts_lex_modes")
	if block == "" {
		return fmt.Errorf("ts_lex_modes not found")
	}

	re := regexp.MustCompile(`\[(\d+)\]\s*=\s*\{\.lex_state\s*=\s*(\d+)(?:,\s*\.external_lex_state\s*=\s*(\d+))?\}`)
	modes := make([]LexModeEntry, g.StateCount)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx, _ := strconv.Atoi(m[1])
		lexState, _ := strconv.Atoi(m[2])
		var extState int
		if m[3] != "" {
			extState, _ = strconv.Atoi(m[3])
		}
		if idx < len(modes) {
			modes[idx] = LexModeEntry{
				LexState:         uint16(lexState),
				ExternalLexState: uint16(extState),
			}
		}
	}
	g.LexModes = modes
	return nil
}

// extractPublicSymbolMap extracts ts_symbol_map[].
func extractPublicSymbolMap(g *Grammar, src string) {
	block := extractArrayBlock(src, "ts_symbol_map")
	if block == "" {
		return
	}

	re := regexp.MustCompile(`\[(\w+)\]\s*=\s*(\w+)`)
	mapping := make([]uint16, g.SymbolCount)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx := g.resolveSymbolIndex(m[1])
		val := g.resolveSymbolValue(m[2])
		if idx >= 0 && idx < g.SymbolCount {
			mapping[idx] = val
		}
	}
	g.PublicSymbolMap = mapping
}

// extractExternalScannerSymbolMap extracts ts_external_scanner_symbol_map[].
// Maps external token index → grammar symbol.
func extractExternalScannerSymbolMap(g *Grammar, src string) {
	if g.ExternalTokenCount == 0 {
		return
	}

	// Build the external token enum for resolving names like ts_external_token__descendant_operator.
	extTokenEnum := extractEnum(src, "ts_external_scanner_symbol_identifiers")

	block := extractArrayBlock(src, "ts_external_scanner_symbol_map")
	if block == "" {
		return
	}

	symbolMap := make([]uint16, g.ExternalTokenCount)

	// Match entries: [ts_external_token_name] = sym_name  OR  [0] = sym_name
	re := regexp.MustCompile(`\[(\w+)\]\s*=\s*(\w+)`)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		tokenIdx := resolveEnumOrInt(m[1], extTokenEnum)
		symVal := g.resolveSymbolValue(m[2])
		if tokenIdx >= 0 && tokenIdx < g.ExternalTokenCount {
			symbolMap[tokenIdx] = symVal
		}
	}

	g.ExternalSymbolMap = symbolMap
}

// extractExternalScannerStates extracts ts_external_scanner_states[][].
// The flat bool array has layout: [extLexState * extTokenCount + tokenIdx].
func extractExternalScannerStates(g *Grammar, src string) {
	if g.ExternalTokenCount == 0 {
		return
	}

	// Build the external token enum for resolving named indices.
	extTokenEnum := extractEnum(src, "ts_external_scanner_symbol_identifiers")

	block := extractArrayBlock(src, "ts_external_scanner_states")
	if block == "" {
		return
	}

	// Match state entries: [N] = { ... } where N is a number.
	re := regexp.MustCompile(`\[(\d+)\]\s*=\s*\{([^}]*)\}`)
	var maxState int
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		idx, _ := strconv.Atoi(m[1])
		if idx > maxState {
			maxState = idx
		}
	}

	states := make([]bool, (maxState+1)*g.ExternalTokenCount)
	for _, m := range re.FindAllStringSubmatch(block, -1) {
		stateIdx, _ := strconv.Atoi(m[1])
		body := m[2]
		// Match token entries: [token_name_or_number] = true
		tokenRe := regexp.MustCompile(`\[(\w+)\]\s*=\s*true`)
		for _, tm := range tokenRe.FindAllStringSubmatch(body, -1) {
			tokenIdx := resolveEnumOrInt(tm[1], extTokenEnum)
			if tokenIdx >= 0 {
				idx := stateIdx*g.ExternalTokenCount + tokenIdx
				if idx < len(states) {
					states[idx] = true
				}
			}
		}
	}
	g.ExternalScannerStates = states
}

// resolveEnumOrInt resolves a C identifier to an integer, trying numeric
// parsing first, then looking up in the enum map.
func resolveEnumOrInt(name string, enumMap map[string]int) int {
	if v, err := strconv.Atoi(name); err == nil {
		return v
	}
	if v, ok := enumMap[name]; ok {
		return v
	}
	return -1
}

// --- Enum parsing ---

// extractEnum parses a C enum block and returns a name -> value map.
func extractEnum(src, enumName string) map[string]int {
	result := make(map[string]int)

	// Find the enum block.
	re := regexp.MustCompile(`enum\s+` + enumName + `\s*\{([^}]*)\}`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return result
	}

	body := m[1]
	// Parse entries: name = value,
	entryRe := regexp.MustCompile(`(\w+)\s*=\s*(\d+)`)
	for _, em := range entryRe.FindAllStringSubmatch(body, -1) {
		name := em[1]
		val, _ := strconv.Atoi(em[2])
		result[name] = val
	}

	return result
}

// extractFieldEnum parses field enum definitions.
// Tree-sitter generates: enum ts_field_identifiers { field_key = 1, ... }
// or sometimes just enum { field_key = 1, ... }
func extractFieldEnum(src string) map[string]int {
	result := make(map[string]int)

	// Try named enum first.
	named := extractEnum(src, "ts_field_identifiers")
	if len(named) > 0 {
		return named
	}

	// Fallback: search for field_xxx = N patterns in any enum.
	re := regexp.MustCompile(`(field_\w+)\s*=\s*(\d+)`)
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		val, _ := strconv.Atoi(m[2])
		result[m[1]] = val
	}

	return result
}

// --- Helper functions ---

// extractArrayBlock finds the block for a named C static array, returning
// the content between the outermost braces.
func extractArrayBlock(src, name string) string {
	// Find the declaration.
	idx := strings.Index(src, name)
	if idx < 0 {
		return ""
	}

	// Find the opening brace.
	start := strings.Index(src[idx:], "{")
	if start < 0 {
		return ""
	}
	start += idx

	end := findMatchingBrace(src, start)
	if end < 0 {
		return ""
	}
	return src[start+1 : end]
}

// findMatchingBrace finds the closing brace that matches the opening brace
// at position start. Handles character literals ('x'), string literals ("x"),
// and nested braces correctly.
func findMatchingBrace(src string, start int) int {
	depth := 0
	i := start
	for i < len(src) {
		ch := src[i]
		switch ch {
		case '\'':
			// Skip character literal: 'x', '\x', '\'' etc.
			// A C char literal is: ' <char-or-escape> '
			// We need to find the matching closing quote.
			i++ // past opening quote
			if i < len(src) && src[i] == '\\' {
				i += 2 // skip backslash AND the escaped character (handles '\'' etc.)
			} else {
				i++ // skip the literal character
			}
			// i should now point at the closing quote; if not, scan for it
			// (handles multi-char escape sequences like '\x1F')
			for i < len(src) && src[i] != '\'' {
				i++
			}
		case '"':
			// Skip string literal.
			i++
			for i < len(src) {
				if src[i] == '\\' {
					i++ // skip escaped char
				} else if src[i] == '"' {
					break
				}
				i++
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
		i++
	}
	return -1
}

// resolveSymbolIndex converts a symbol name (like "ts_builtin_sym_end" or
// "anon_sym_LBRACE") to an integer index using the grammar's enum map.
func (g *Grammar) resolveSymbolIndex(name string) int {
	// Try as a number first.
	if v, err := strconv.Atoi(name); err == nil {
		return v
	}
	// Look up in the symbol enum map.
	if g.symbolEnum != nil {
		if v, ok := g.symbolEnum[name]; ok {
			return v
		}
	}
	return -1
}

// resolveSymbolValue converts a symbol reference to a uint16 using the enum map.
func (g *Grammar) resolveSymbolValue(name string) uint16 {
	// Try as a number first.
	if v, err := strconv.Atoi(name); err == nil {
		return uint16(v)
	}
	if g.symbolEnum != nil {
		if v, ok := g.symbolEnum[name]; ok {
			return uint16(v)
		}
	}
	return 0
}

// resolveFieldID converts a field name reference to a uint16 using the field enum map.
func (g *Grammar) resolveFieldID(name string) uint16 {
	if v, err := strconv.Atoi(name); err == nil {
		return uint16(v)
	}
	if g.fieldEnum != nil {
		if v, ok := g.fieldEnum[name]; ok {
			return uint16(v)
		}
	}
	return 0
}

// extractInt extracts the first integer from a string matching a regex.
func extractInt(s, pattern string) int {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if m == nil || len(m) < 2 {
		return 0
	}
	v, _ := strconv.Atoi(m[1])
	return v
}

// unescapeCString converts C string escapes to Go equivalents.
func unescapeCString(s string) string {
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}

// tokenizeLine splits a C source line into tokens (numbers, macro calls, symbol refs).
func tokenizeLine(line string) []string {
	// Remove comments.
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = line[:idx]
	}

	var tokens []string
	re := regexp.MustCompile(`\w+\([^)]*\)|\w+`)
	for _, m := range re.FindAllString(line, -1) {
		tokens = append(tokens, m)
	}
	return tokens
}

// parseTableToken converts a token from the parse table to a uint16.
func (g *Grammar) parseTableToken(tok string) (uint16, bool) {
	// ACTIONS(N) -> N
	if strings.HasPrefix(tok, "ACTIONS(") {
		s := tok[8 : len(tok)-1]
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil {
			return uint16(v), true
		}
	}
	// STATE(N) -> N
	if strings.HasPrefix(tok, "STATE(") {
		s := tok[6 : len(tok)-1]
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil {
			return uint16(v), true
		}
	}
	// Bare number
	v, err := strconv.Atoi(tok)
	if err == nil {
		return uint16(v), true
	}
	// Symbol enum name
	if g.symbolEnum != nil {
		if val, ok := g.symbolEnum[tok]; ok {
			return uint16(val), true
		}
	}
	return 0, false
}
