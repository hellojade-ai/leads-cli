#!/usr/bin/env bash
# Send a batch of leads, branching on the exit code the way a real job should.
#
#   HELLOJADE_API_KEY=... ./backfill.sh leads.psv          # dry run
#   HELLOJADE_API_KEY=... ./backfill.sh leads.psv --send   # actually sends
#
# leads.psv is PIPE-separated: id|first|last|phone
#
#   A-99812|Dana|Whitfield|(630) 555-0142
#
# Pipe, not tab, on purpose: bash treats tab as IFS *whitespace*, so a run of
# two tabs collapses into one delimiter and an empty column silently shifts
# every field after it. A non-whitespace IFS character does not collapse. This
# is not a hellojade rule — it is a shell trap that will hand the API a lead
# with the surname in the phone field and no way to notice.
#
# Doing a real backfill? ASK HELLOJADE FIRST. The per-key budget is 600/min,
# but drip-feeding thousands of rows to stay under it takes days and looks
# like an incident from their side. They will raise the limit for a window.
set -uo pipefail

FILE=${1:?usage: backfill.sh leads.psv [--send]}
DRY=--dry-run
[ "${2:-}" = "--send" ] && DRY=""

# Namespace every key with something only you use. Dedupe is scoped to the
# TENANT: a bare id can collide with another source's lead under the same
# customer, and yours is then silently never stored.
NS=acme-leads

sent=0; requeue=0; alert=0

while IFS='|' read -r id first last phone; do
  [ -z "${id:-}" ] && continue
  case "$id" in \#*) continue ;; esac

  # The SAME key on every attempt, forever, for this lead. Never a timestamp,
  # never a fresh UUID per attempt — that is what makes a retry safe.
  #
  # </dev/null so a subcommand can never eat the rows this loop is reading.
  hellojade leads submit $DRY --quiet \
    --external-id "${NS}:${id}" \
    --first-name "$first" --last-name "$last" --phone "$phone" \
    --request-id "${NS}/${id}" </dev/null
  code=$?

  case $code in
    0)
      # 202 accepted or 200 duplicate. Both mean hellojade has it on disk.
      sent=$((sent + 1)) ;;
    2|3|4)
      # Usage, a bad key, or a body the server will keep rejecting. Retrying
      # unchanged only burns rate limit. A human has to look at this.
      echo "ALERT   $id — exit $code (usage / auth / rejected)" >&2
      alert=$((alert + 1)) ;;
    5|6|7)
      # Rate limit, their server, or the network. The lead is not lost:
      # resend it later with the SAME idempotency key.
      echo "REQUEUE $id — exit $code (rate-limited / server / network)" >&2
      requeue=$((requeue + 1)) ;;
    *)
      echo "ALERT   $id — unexpected exit $code" >&2
      alert=$((alert + 1)) ;;
  esac
done < "$FILE"

echo "sent=$sent requeue=$requeue alert=$alert"
# Non-zero only when a human needs to look at something. A requeue is normal.
[ "$alert" -eq 0 ]
