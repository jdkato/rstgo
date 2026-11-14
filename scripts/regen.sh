#!/bin/sh
# Regenerate the expected test outputs with the reference implementation
# (Docutils; `pip install docutils==0.23`).  Run from anywhere:
#
#   scripts/regen.sh            # everything
#   scripts/regen.sh fixtures   # only testdata/fixtures
#   scripts/regen.sh suites     # only testdata/suites
set -e
cd "$(dirname "$0")/.."
exec python3 scripts/regen.py "$@"
