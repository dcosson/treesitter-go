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

	key = impl SUBSEP lang SUBSEP size
	sum[key] += ns
	cnt[key]++

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
	printf "%-14s %-10s %12s %12s %12s %12s %10s\n", \
		"Language", "Size", "Go (total)", "Ref (total)", "Go (parse)", "Ref (parse)", "Go vs Ref"
	printf "%-14s %-10s %12s %12s %12s %12s %10s\n", \
		"--------", "----", "----------", "-----------", "----------", "-----------", "---------"

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

			if (size == "overhead") {
				printf "%-14s %-10s %12s %12s %12s %12s %10s\n", \
					lang, size, fmt_ns(go_avg), fmt_ns(ref_avg), "-", "-", "-"
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

				printf "%-14s %-10s %12s %12s %12s %12s %10s\n", \
					lang, size, fmt_ns(go_avg), fmt_ns(ref_avg), \
					fmt_ns(go_parse), fmt_ns(ref_parse), ratio_str
			}
		}
	}
	printf "\n"
	printf "Go vs Ref: >1x means Go is faster, <1x means Go is slower.\n"
	printf "Parse times = total - overhead (subprocess startup cost removed).\n"
	printf "\n"
}

function fmt_ns(ns) {
	if (ns >= 1e9) return sprintf("%.2fs", ns / 1e9)
	if (ns >= 1e6) return sprintf("%.1fms", ns / 1e6)
	if (ns >= 1e3) return sprintf("%.1fus", ns / 1e3)
	return sprintf("%.0fns", ns)
}
' "$file"
