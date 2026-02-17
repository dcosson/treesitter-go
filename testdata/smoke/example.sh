#!/bin/bash

count_files() {
  local dir="$1"
  local count
  count=$(find "$dir" -type f | wc -l)
  echo "$count"
}

process_dir() {
  local dir="$1"
  if [ ! -d "$dir" ]; then
    echo "Error: $dir is not a directory"
    return 1
  fi
  local num
  num=$(count_files "$dir")
  echo "Directory $dir has $num files"
}

for dir in "$@"; do
  process_dir "$dir"
done
