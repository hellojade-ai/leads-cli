# Examples

Runnable shell scripts. Each one is executable and takes no arguments beyond
what it documents at the top.

| file | what it shows |
|---|---|
| [`check-key.sh`](check-key.sh) | the key check, and what each exit code means. **Run this first** |
| [`lead.json`](lead.json) | a complete lead envelope, every field the API models |
| [`submit.sh`](submit.sh) | one lead from flags, and the same lead from JSON, both as a dry run first |
| [`backfill.sh`](backfill.sh) + [`leads.psv`](leads.psv) | a batch loop: stable namespaced idempotency keys, exit-code branching, requeue vs. alert. Pipe-separated, not tab — read the note at the top of the script |
| [`vocabulary-snapshot.sh`](vocabulary-snapshot.sh) | taking a build-time snapshot of `project_area` without hard-coding it |

All of them read `HELLOJADE_API_KEY` from the environment and honor
`HELLOJADE_BASE_URL`, so you can point them at a stub server while you work:

```sh
export HELLOJADE_BASE_URL=http://127.0.0.1:8080
```

> **Do not run `submit.sh` or `backfill.sh` against production with made-up
> names and phone numbers.** A real salesperson gets the call, and someone has
> to delete the row by hand. They default to `--dry-run` for that reason. Use
> `check-key.sh` for a live round trip, or ask hellojade for a sandbox key.
