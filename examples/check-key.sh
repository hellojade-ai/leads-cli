#!/usr/bin/env bash
# Prove your API key works without creating a lead.
#
#   HELLOJADE_API_KEY=... ./check-key.sh
#
# This is the ONE request that is safe to make against production while you
# are building: the server authenticates before it validates, so an empty body
# with a real key is rejected by the validator with a 422 — which proves the
# key and stores nothing. Nothing is delivered, emailed, or written to a CRM.
set -uo pipefail

hellojade auth check
code=$?

case $code in
  0) echo "the key is valid and active" ;;
  3) echo "401: missing, mistyped, revoked, or you are on the wrong host." >&2
     echo "     check the value for a trailing newline, then ask hellojade if it is active." >&2 ;;
  5) echo "429: the per-IP budget, which is applied BEFORE authentication." >&2
     echo "     this says nothing about your key. Wait a second and repeat." >&2 ;;
  7) echo "transport failure. Check for http:// first — nothing listens on port 80," >&2
     echo "     so it fails with a connection error, not a redirect." >&2 ;;
  *) echo "unexpected exit $code — run 'hellojade help exit-codes'" >&2 ;;
esac

exit $code
