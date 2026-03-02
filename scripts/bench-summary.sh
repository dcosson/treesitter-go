#!/usr/bin/env bash
# Summarize BenchmarkCompare results with overhead-adjusted parse times.
# Usage: scripts/bench-summary.sh [file]    (default: testdata/bench-results.txt)
set -euo pipefail

file="${1:-testdata/bench-results.txt}"

if [ ! -f "$file" ]; then
	echo "No benchmark results file: $file" >&2
	exit 1
fi

awk '
/^BenchmarkCompare\// {
	n = split($1, parts, "/")
	impl = parts[2]
	lang = parts[3]
	size = parts[4]
	sub(/-[0-9]+$/, "", size)

	ns = $3 + 0
	bop = $7 + 0

	key = impl SUBSEP lang SUBSEP size
	sum[key] += ns
	cnt[key]++
	mem[key] += bop

	if (!(lang in seen_lang)) {
		seen_lang[lang] = 1
		lang_order[++nlang] = lang
	}
	lskey = lang SUBSEP size
	if (!(lskey in seen_ls)) {
		seen_ls[lskey] = 1
		if (!(size in size_idx)) {
			size_idx[size] = ++nsize
			size_order[nsize] = size
		}
	}
}

END {
	if (nlang == 0) {
		print "No BenchmarkCompare results found in " FILENAME > "/dev/stderr"
		exit 1
	}

	printf "\n"
	printf "%-14s %-10s %12s %12s %12s %12s %10s %10s %10s %10s\n", \
		"Language", "Size", "Go (total)", "Ref (total)", "Go (parse)", "Ref (parse)", "Go vs Ref", "Go mem", "Ref mem", "Go/Ref mem"
	printf "%-14s %-10s %12s %12s %12s %12s %10s %10s %10s %10s\n", \
		"--------", "----", "----------", "-----------", "----------", "-----------", "---------", "------", "-------", "----------"

	for (li = 1; li <= nlang; li++) {
		lang = lang_order[li]

		go_oh_key = "go" SUBSEP lang SUBSEP "overhead"
		ref_oh_key = "ref" SUBSEP lang SUBSEP "overhead"
		go_overhead = (cnt[go_oh_key] > 0) ? sum[go_oh_key] / cnt[go_oh_key] : 0
		ref_overhead = (cnt[ref_oh_key] > 0) ? sum[ref_oh_key] / cnt[ref_oh_key] : 0

		for (si = 1; si <= nsize; si++) {
			size = size_order[si]

			go_key = "go" SUBSEP lang SUBSEP size
			ref_key = "ref" SUBSEP lang SUBSEP size

			if (cnt[go_key] == 0 && cnt[ref_key] == 0) continue

			go_avg = (cnt[go_key] > 0) ? sum[go_key] / cnt[go_key] : 0
			ref_avg = (cnt[ref_key] > 0) ? sum[ref_key] / cnt[ref_key] : 0
			go_mem = (cnt[go_key] > 0) ? mem[go_key] / cnt[go_key] : 0
			ref_mem = (cnt[ref_key] > 0) ? mem[ref_key] / cnt[ref_key] : 0

			if (size == "overhead") {
					printf "%-14s %-10s %12s %12s %12s %12s %10s %10s %10s %10s\n", \
						lang, size, fmt_ns(go_avg), fmt_ns(ref_avg), "-", "-", "-", \
						fmt_bytes(go_mem), fmt_bytes(ref_mem), "-"
				} else {
					go_parse = go_avg - go_overhead
					ref_parse = ref_avg - ref_overhead
					if (go_parse < 0) go_parse = 0
					if (ref_parse < 0) ref_parse = 0

					if (go_parse > 0 && ref_parse > 0) {
						ratio = ref_parse / go_parse
						ratio_str = sprintf("%.2fx", ratio)
					} else {
						ratio_str = "-"
					}
					if (go_mem > 0 && ref_mem > 0) {
						mem_ratio = go_mem / ref_mem
						mem_ratio_str = sprintf("%.2fx", mem_ratio)
					} else {
						mem_ratio_str = "-"
					}

					printf "%-14s %-10s %12s %12s %12s %12s %10s %10s %10s %10s\n", \
						lang, size, fmt_ns(go_avg), fmt_ns(ref_avg), \
						fmt_ns(go_parse), fmt_ns(ref_parse), ratio_str, \
						fmt_bytes(go_mem), fmt_bytes(ref_mem), mem_ratio_str
				}
			}
		}
	printf "\n"
	printf "Go vs Ref: >1x means Go is faster, <1x means Go is slower.\n"
	printf "Go/Ref mem: >1x means Go uses more memory, <1x means Go uses less.\n"
	printf "Parse times = total - overhead (subprocess startup cost removed).\n"
	printf "\n"

	# Per-language summary table: average Go vs Ref CPU and memory multipliers.
	# Computed from summed parse times and memory across all sizes per language.
	printf "%-14s %10s %10s\n", "Language", "Go vs Ref", "Go/Ref mem"
	printf "%-14s %10s %10s\n", "--------", "---------", "----------"

	total_go_parse = 0
	total_ref_parse = 0
	total_go_mem = 0
	total_ref_mem = 0
	lang_count = 0

	for (li = 1; li <= nlang; li++) {
		lang = lang_order[li]

		go_oh_key = "go" SUBSEP lang SUBSEP "overhead"
		ref_oh_key = "ref" SUBSEP lang SUBSEP "overhead"
		go_overhead = (cnt[go_oh_key] > 0) ? sum[go_oh_key] / cnt[go_oh_key] : 0
		ref_overhead = (cnt[ref_oh_key] > 0) ? sum[ref_oh_key] / cnt[ref_oh_key] : 0

		lang_go_parse = 0
		lang_ref_parse = 0
		lang_go_mem = 0
		lang_ref_mem = 0
		lang_sizes = 0

		for (si = 1; si <= nsize; si++) {
			size = size_order[si]
			if (size == "overhead") continue

			go_key = "go" SUBSEP lang SUBSEP size
			ref_key = "ref" SUBSEP lang SUBSEP size

			if (cnt[go_key] == 0 || cnt[ref_key] == 0) continue

			go_avg = sum[go_key] / cnt[go_key]
			ref_avg = sum[ref_key] / cnt[ref_key]
			go_m = mem[go_key] / cnt[go_key]
			ref_m = mem[ref_key] / cnt[ref_key]

			go_p = go_avg - go_overhead
			ref_p = ref_avg - ref_overhead
			if (go_p < 0) go_p = 0
			if (ref_p < 0) ref_p = 0

			lang_go_parse += go_p
			lang_ref_parse += ref_p
			lang_go_mem += go_m
			lang_ref_mem += ref_m
			lang_sizes++
		}

		if (lang_sizes > 0 && lang_go_parse > 0 && lang_ref_parse > 0) {
			cpu_ratio = lang_ref_parse / lang_go_parse
			cpu_str = sprintf("%.2fx", cpu_ratio)
		} else {
			cpu_str = "-"
		}
		if (lang_sizes > 0 && lang_go_mem > 0 && lang_ref_mem > 0) {
			mem_r = lang_go_mem / lang_ref_mem
			mem_str = sprintf("%.2fx", mem_r)
		} else {
			mem_str = "-"
		}

		printf "%-14s %10s %10s\n", lang, cpu_str, mem_str

		total_go_parse += lang_go_parse
		total_ref_parse += lang_ref_parse
		total_go_mem += lang_go_mem
		total_ref_mem += lang_ref_mem
		if (lang_sizes > 0) lang_count++
	}

	printf "%-14s %10s %10s\n", "--------", "---------", "----------"
	if (total_go_parse > 0 && total_ref_parse > 0) {
		total_cpu = total_ref_parse / total_go_parse
		total_cpu_str = sprintf("%.2fx", total_cpu)
	} else {
		total_cpu_str = "-"
	}
	if (total_go_mem > 0 && total_ref_mem > 0) {
		total_mem = total_go_mem / total_ref_mem
		total_mem_str = sprintf("%.2fx", total_mem)
	} else {
		total_mem_str = "-"
	}
	printf "%-14s %10s %10s\n", "TOTAL", total_cpu_str, total_mem_str
	printf "\n"
}

function fmt_ns(ns) {
	if (ns >= 1e9) return sprintf("%.2fs", ns / 1e9)
	if (ns >= 1e6) return sprintf("%.1fms", ns / 1e6)
	if (ns >= 1e3) return sprintf("%.1fus", ns / 1e3)
	return sprintf("%.0fns", ns)
}

function fmt_bytes(b) {
	if (b >= 1048576) return sprintf("%.1fMB", b / 1048576)
	if (b >= 1024) return sprintf("%.1fKB", b / 1024)
	return sprintf("%.0fB", b)
}
' "$file"
