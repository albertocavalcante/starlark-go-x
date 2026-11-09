# Copyright 2026 Alberto Cavalcante. See LICENSE for licensing terms.
FILENAME == ARGV[1] {
  if (NF != 1 || $0 in expected) bad = 1
  expected[$0] = 1
  expected_count++
  next
}
/^Benchmark/ {
  name = $1
  sub(/-[0-9]+$/, "", name)
  valid = (NF >= 8 && NF % 2 == 0 && $2 ~ /^[1-9][0-9]*$/)
  for (unit in units) delete units[unit]
  for (i = 3; i <= NF; i += 2) {
    unit = $(i + 1)
    if ($i !~ /^[0-9]+([.][0-9]*)?([eE][+-]?[0-9]+)?$/ ||
        unit !~ /^(ns\/op|MB\/s|B\/op|allocs\/op)$/ || units[unit]++) valid = 0
  }
  if (!units["ns/op"] || !units["B/op"] || !units["allocs/op"]) valid = 0
  if (!(name in expected) || !valid) {
    print "unexpected or malformed benchmark row: " $0 > "/dev/stderr"
    bad = 1
    next
  }
  count[name]++
}
END {
  if (!expected_count) bad = 1
  for (name in expected) if (count[name] != 12) {
    print "incomplete benchmark: " name > "/dev/stderr"
    bad = 1
  }
  exit bad
}
