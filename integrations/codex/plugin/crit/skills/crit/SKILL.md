---
name: crit
description: Review code changes, PRs, commit ranges, or explicitly named files/plans with crit inline comments. Use when the user invokes $crit or asks for structured human feedback on code or a specific review target.
---

# Review with Crit

Review and revise code changes or a plan using `crit` for inline comment review.

## Step 1: Determine review mode

Pick whichever applies — don't ask for confirmation:

1. **PR or commit range** — user asked to review a specific GitHub PR or commit range → `crit --pr <num|url>` or `crit --range <baseSHA>..<headSHA>`. Boots crit in *range mode*, scoping the review to a fixed range of commits rather than the working tree.
2. **Explicit file/plan argument** — user specified a file, directory, or plan path (e.g. `$crit my-plan.md`) → `crit <path>`
3. **Default `$crit` / branch review** — otherwise → bare `crit`. Auto-detects uncommitted changes or branch-vs-default-branch diff. Works on clean branches.

Do **not** infer a recent plan file for bare `$crit`. Bare `$crit` should behave like Claude Code's `/crit`: review the current code changes in git mode unless the user explicitly names a file, plan, PR, or range.

## Step 2: Launch crit and block until review completes

**CRITICAL — you MUST run this step. Do NOT skip it. Do NOT proceed without it.**

Run `crit` in the foreground and block until it exits:

```bash
crit <plan-file>   # specific file
crit               # git mode
```

If a crit server is already running from earlier in this conversation, `crit` automatically connects to it. Starting from scratch, it spawns the daemon, opens the browser, and blocks until the user clicks "Finish Review".

`crit` prints the review URL on startup (e.g. `Started crit daemon at http://localhost:<port>`). Relay it verbatim:

> **"Crit is open at http://localhost:<port>. Leave inline comments, then click Finish Review."**

**Do NOT proceed until `crit` completes.** Do NOT ask the user to type anything. Do NOT read the review file early. Wait for the foreground command to finish — that is how you know the human is done reviewing.

## Step 3: Check the review result

When `crit` completes, inspect its stdout JSON first.

If `"approved": true`, the review has no unresolved comments. Tell the user no changes were requested and stop. Do **not** read `review_file` on approval; Crit may delete it immediately when `cleanup_on_approve` is enabled.

If `"approved": false`, stdout includes `review_file`. Read that file.

The file contains structured JSON. Three comment types:
- `review_comments` (top-level, `r_`-prefixed IDs) — general feedback
- File comments (per-file `comments` array, no `start_line`/`end_line`) — about the file as a whole
- Line comments (per-file `comments` array, with `start_line`/`end_line`) — about specific lines

Identify all comments where `resolved` is `false` or missing.

When a comment has these fields:
- `quote`: the specific text the reviewer selected — focus your changes on the quoted text rather than the entire line range
- `anchor`: use it to locate the current position of the content; line numbers may be stale after edits
- `drifted: true`: original content was removed or heavily rewritten — line numbers are approximate at best

## Step 4: Address each review comment

For each unresolved comment:

1. Understand what the comment asks for
2. If it contains a suggestion block, apply that specific change
3. Revise the referenced file (plan or code file from the diff)
4. Reply with what you did: `crit comment --reply-to <id> --author 'Codex' '<what you did>'` (reply bodies support markdown)
5. **Do not pass `--resolve`.** Resolving is the reviewer's call. Only add `--resolve` if the user explicitly asks.

Editing the plan file triggers Crit's live reload — the user sees changes in the browser immediately.

### When replying to multiple comments

Use `--json` for a single bulk call instead of one invocation per comment:

```bash
echo '[
  {"reply_to": "c_a1b2c3", "body": "Fixed"},
  {"reply_to": "c_d4e5f6", "body": "Refactored as suggested"}
]' | crit comment --json --author 'Codex'
```

If the file has no unresolved comments, inform the user no changes were requested and stop.

## Step 5: Signal completion and start next round

**CRITICAL — you MUST run this step. Do NOT skip it. Do NOT proceed without it.**

When Step 2's `crit` command exits with feedback, it prints `Next round: crit <args>` to stdout. Run that command verbatim — the daemon is keyed by args, so mismatched args spawn a new daemon instead of reconnecting.

On subsequent calls, `crit` automatically signals round-complete first, then blocks until the next "Finish Review" click.

Tell the user: **"Changes applied. Review the diff in your browser and click Finish Review when ready."**

**Do NOT proceed until `crit` completes.** When it does, return to Step 3. If the user finishes with zero comments, the review is approved — stop the loop and proceed.

## Sharing

If the user asks for a URL, a shareable link, or to share the review:

```bash
crit share <file>
```

**Always relay the full output to the user** — copy the URL directly into your response. Don't make them dig through tool output.

To remove a shared review:

```bash
crit unpublish
```

### QR codes

Only use `--qr` in real terminal environments with monospace rendering. Skip it in mobile apps or web chat UIs — Unicode block characters won't render.

```bash
crit share --qr <file>
```
