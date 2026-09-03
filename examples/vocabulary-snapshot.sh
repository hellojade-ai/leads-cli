#!/usr/bin/env bash
# Take a build-time snapshot of the project_area vocabulary.
#
#   ./vocabulary-snapshot.sh [outfile]
#
# The vocabulary grows by database insert on the server, with no deploy on
# their side, so it must never be hard-coded in your source. If your build
# needs a static list, fetch it at BUILD time like this and treat it as a
# cache, not as the truth.
#
# An unrecognized term is not an error: the server stores your raw value,
# flags it, and counts it so the vocabulary grows from real traffic. Sending
# "roofing" is better than guessing at "roof" or dropping the field.
set -euo pipefail

out=${1:-project-areas.txt}

# No API key is needed for this endpoint.
hellojade vocabulary --areas > "$out"
echo "wrote $(wc -l < "$out") terms to $out"

# The full record, with each term's confirmed/proposed status:
hellojade vocabulary --json > "${out%.txt}.json"
echo "wrote ${out%.txt}.json"
