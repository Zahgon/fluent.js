#!/bin/sh
# Regenerates internal/icudata/icu.zip from the pinned Node image; run in fluent-dev (node + python3).
set -eu
cd "$(dirname "$0")"
mkdir -p out
python3 finddat.py /usr/local/bin/node out/icudt78l.dat
python3 mkicu.py out/icudt78l.dat "$1" /usr/local/bin/node
