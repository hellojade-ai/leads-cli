#!/usr/bin/env bash
# Submit one lead — from flags, and the same lead from JSON.
#
#   HELLOJADE_API_KEY=... ./submit.sh          # dry run: prints, sends nothing
#   HELLOJADE_API_KEY=... ./submit.sh --send   # actually sends
#
# Defaults to --dry-run on purpose. Do NOT send a made-up name and phone
# number to production: a real salesperson gets the call. Ask hellojade for a
# sandbox key if you need a live round trip.
set -uo pipefail

DRY=--dry-run
[ "${1:-}" = "--send" ] && DRY=""

# Rule 2 and rule 3: the Idempotency-Key is YOUR OWN stable id for the lead,
# namespaced to you. --external-id supplies it. A bare "A-99812" would share
# an idempotency namespace with every other source posting to this customer.
ID='acme-leads:A-99812'

echo "== from flags =="
hellojade leads submit $DRY \
  --external-id "$ID" \
  --first-name Dana \
  --last-name Whitfield \
  --phone '(630) 555-0142' \
  --email dana.whitfield@example.com \
  --street-address '418 N Maple St' \
  --city Naperville --state IL --zip 60540 \
  --project-area roof \
  --project-service replacement \
  --project-material 'asphalt shingle' \
  --project-details 'Hail damage on the south slope, insurance claim already filed.' \
  --cost 555.55 \
  --extra partner_job_id=XZ-1 \
  --request-id "acme-leads/A-99812"
flags_code=$?

echo
echo "== the same lead from JSON =="
# Unmodeled top-level keys in the file (partner_job_id) are preserved verbatim
# by the server under `extra`, and come back as an extra_fields_preserved flag.
# Flags are NOT errors.
hellojade leads submit $DRY --json-file lead.json
file_code=$?

echo
echo "== from stdin, with a flag overriding one field =="
cat lead.json | hellojade leads submit --dry-run --json-file - --city Aurora >/dev/null \
  && echo "stdin + override: ok (always a dry run here)"

echo
echo "exit codes: flags=$flags_code json=$file_code"
# 0 covers BOTH 202 accepted and 200 duplicate. A duplicate is what a retry is
# supposed to produce, and it carries the original event_id.
exit $(( flags_code > file_code ? flags_code : file_code ))
