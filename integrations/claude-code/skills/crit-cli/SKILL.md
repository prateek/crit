---
name: crit-cli
description: Use when an agent needs to author or reply to crit inline comments programmatically (including multi-agent workflows commenting on shared code/plans/docs/proposals), publish or unpublish a crit review with crit share, sync a crit review to or from a GitHub PR or GitLab MR, or read/interpret a crit review JSON file. Covers crit comment, crit share, crit unpublish, crit pull, crit push, review file format, and resolution workflow. Not for invoking an interactive review loop — that's the `/crit` command.
user-invocable: false
---

# Crit CLI Reference

> If a plan was just written and the user said `/crit` or `crit`, invoke the `/crit` command — do not use this reference skill. This skill covers CLI operations like `crit comment`, `crit pull/push`, and `crit share`.

Comments have three scopes:

- **Line comments** (`scope: "line"`) — tied to specific lines, stored in `files.<path>.comments`
- **File comments** (`scope: "file"`) — about a file overall, stored in `files.<path>.comments` with `start_line: 0`
- **Review comments** (`scope: "review"`) — general feedback, stored in the top-level `review_comments` array

The review file path is shown by `crit status`.

## Reading comments

When `crit` completes a review round, read **stdout** and follow its instructions. Unresolved comments are often embedded in that prompt as JSON. Check **stderr** for `approved: true` or `approved: false`.

When you need to read comments separately:

```bash
crit comments            # human-readable, unresolved only (default)
crit comments --json     # flat JSON for agents
crit comments --all      # include resolved comments
crit comments --plan <slug>   # plan reviews
crit comments [path]     # explicit review.json or .crit directory
```

Review-level comments are listed first — easy to miss in raw `review.json`. Uses the same review resolution as `crit comment` (`--output`, `--plan`, daemon session).

## Multiple active sessions

When more than one review session matches the current directory and branch, headless commands (`crit comment`, `crit comments`, `crit share`, `crit push`, `crit pull`) refuse to guess. Run `crit status` (or `crit status --json`) to list every active session, then target the intended review with `--session <id>`:

```bash
crit comment --session <id> --author <name> <path>:<line> <body>
crit comment --session <id> --json --file comments.json --author <name>
crit comments --session <id>
crit share --session <id> <file>
crit push --session <id>
crit pull --session <id>
```

The JSON status output exposes the candidates in `sessions`.



<important if="you are reading or parsing the review file">

```json
{
  "review_comments": [
    {
      "id": "r_f1e2d3",
      "body": "Overall the architecture looks good",
      "scope": "review",
      "author": "User Name",
      "resolved": false,
      "replies": [
        { "id": "rp_b4a5c6", "body": "Thanks, addressed the minor issues", "author": "Claude" }
      ]
    }
  ],
  "files": {
    "path/to/file.go": {
      "comments": [
        {
          "id": "c_a1b2c3",
          "start_line": 5,
          "end_line": 10,
          "body": "Comment text",
          "quote": "the specific words selected",
          "anchor": "The sessions table needs a complete rewrite...",
          "author": "User Name",
          "resolved": false,
          "replies": [
            { "id": "rp_c7d8e9", "body": "Fixed by extracting to helper", "author": "Claude" }
          ]
        }
      ]
    }
  }
}
```

Field rules:
- `resolved`: `false` or **missing** — both mean unresolved. Only `true` means resolved.
- `quote` (optional): the specific text the reviewer selected — narrows scope within the line range. Focus changes on the quoted text rather than the entire range.
- `anchor` (line comments): full text of the commented lines when placed. When edits shift line numbers, locate content by anchor rather than trusting `start_line`/`end_line`.
- `drifted: true`: original content was removed or heavily rewritten — line numbers are approximate at best.
- Unresolved comments may have `replies` — read them before acting.
</important>

<important if="you are authoring or replying to comments via crit comment">

```bash
# Review-level (general feedback)
crit comment --author 'Claude Code' '<body>'

# File-level (whole file, no line numbers)
crit comment --author 'Claude Code' <path> '<body>'

# Line (single line or range)
crit comment --author 'Claude Code' <path>:<line> '<body>'
crit comment --author 'Claude Code' <path>:<start>-<end> '<body>'

# Reply to an existing comment
crit comment --reply-to <id> --author 'Claude Code' '<body>'
```

Hard rules:
- **Always pass `--author 'Claude Code'`** (or your agent name) so comments are attributed correctly.
- **Always single-quote the body** — double quotes break on backticks and shell metachars.
- **Line numbers reference the file on disk** (1-indexed), not diff line numbers.
- **Reply bodies support markdown** — use code fences and inline code where helpful.
- **Only pass `--resolve` when the user explicitly asks.** Never resolve proactively.
</important>

<important if="you authored comments and are about to tell the user to look at the review">

`crit comments` and the review file are the **store**. The browser renders the
running daemon's **filtered projection** of that store, scoped to the session's
current focus. A comment can be present in both and still render nowhere, so
reading it back with `crit comments` does not prove the reviewer can see it.

Check what the daemon actually serves before handing the review back. Wait for
readiness first, and give `crit comment` a second to land: it writes to disk and
the daemon picks it up on a one-second watcher tick, so an id missing
immediately after the CLI returns proves nothing.

```bash
IDS='c_a1b2c3 c_d4e5f6'                       # every id you just authored
FILE=<repo-relative-path>                     # repeat the file GET per file you touched

PORT=$(crit status --json | jq -r '.daemon.port // empty')
[ -n "$PORT" ] || { echo 'several sessions match; pick one from .sessions[]'; exit 1; }

for _ in $(seq 20); do                        # 503 while the daemon builds the projection
  code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT/api/session")
  case "$code" in
    200) break ;;
    503) sleep 1 ;;
    *) echo "daemon returned $code"; exit 1 ;; # 000 is a dead daemon, not a slow one
  esac
done
[ "$code" = 200 ] || { echo 'daemon never became ready'; exit 1; }

for _ in $(seq 5); do                         # the watcher tick has arbitrary phase
  served=$( { curl -s --get --data-urlencode "path=$FILE" \
                "http://127.0.0.1:$PORT/api/file/comments"
              curl -s "http://127.0.0.1:$PORT/api/comments"
            } | jq -r '.[]?.id, .[]?.replies[]?.id' )
  missing=
  for id in $IDS; do
    printf '%s\n' "$served" | grep -qxF "$id" || missing="$missing $id"
  done
  [ -z "$missing" ] && break
  sleep 1
done
[ -z "$missing" ] || { echo "daemon does not serve:$missing"; exit 1; }
curl -s "http://127.0.0.1:$PORT/api/health"
```

- Check every id you authored, not a sample. Line and file comments come back
  from `/api/file/comments?path=`, review-level comments from `/api/comments`,
  and a `--reply-to` id sits under its parent's `replies`. Match the `id` field
  exactly: a substring search over the body also matches quoted code.
- When `crit comments` lists an id and the daemon still does not serve it after
  the tick, the active focus filters it out; re-author it through the daemon.
- `crit status --json` omits `.daemon.port` while several sessions match the
  branch. Read the port out of `.sessions[]` instead, picking the id you want:
  `crit status` takes no `--session`.
- `/api/health` reports `browser_clients`, a boolean that is true while at
  least one tab holds an `/api/events` connection. Read it as a hint: a tab
  that is loading or reconnecting reads as `false`.
- Reconnecting to a live daemon keeps its focus. Restarting a dead one loses it:
  a `--pr`, `--mr` or `--range` session persists no `cli_args`, so
  `crit --session <id>` comes back in working-tree focus and hides every comment
  scoped to the old focus. Relaunch with the invocation that created the review.

</important>

<important if="you are leaving 3+ comments in one operation">

Use `--json` for atomicity (single write, no partial state) and speed (one process). Two ways to feed the JSON:

```bash
# Short, single-line bodies — pipe via stdin:
echo '[
  {"body": "overall feedback", "scope": "review"},
  {"path": "session.go", "body": "restructure", "scope": "file"},
  {"file": "src/auth.go", "line": 42, "body": "Missing null check"},
  {"file": "src/auth.go", "line": "50-55", "body": "Extract to helper"},
  {"reply_to": "c_a1b2c3", "body": "Fixed — added null check"},
  {"reply_to": "r_f1e2d3", "body": "Done"}
]' | crit comment --json --author 'Claude Code'
```

**Prefer `--file <path>` for any multi-paragraph body.** Shell-quoted JSON breaks the moment a `"body"` string contains a raw newline — JSON forbids them, and the shell happily passes them through. Use the Write tool to author the JSON to a temp file, then point crit at it:

```bash
# After Write-ing /tmp/replies.json:
crit comment --json --file /tmp/replies.json --author 'Claude Code'
```

`--file -` reads stdin (same as omitting the flag).

Per-entry schema:

| Field | Type | Required | Notes |
|---|---|---|---|
| `file` / `path` | string | line/file comments | Relative path. `path` alone (no `line`) → file-level. |
| `line` | int/string | line comments | `42` or `"45-47"` |
| `end_line` | int | optional | Defaults to `line` |
| `body` | string | always | |
| `author` | string | optional | Per-entry override; falls back to `--author` |
| `scope` | string | optional | `"review"` / `"file"` — usually inferred |
| `reply_to` | string | replies | Comment ID (`c_…` or `r_…`) |
| `resolve` | bool | optional | Only when user explicitly asks |

Scope inference (when `scope` omitted): has `reply_to` → reply; no `file`/`path` and no `line` → review-level; `path` but no `line` → file-level; `file`/`path` + `line` → line.
</important>

<important if="crit comment errored with 'comment found in multiple files'">
Comment IDs are unique per session, but the same ID can collide across files. Disambiguate with `--path`:

```bash
crit comment --reply-to c_a1b2c3 --path src/auth.go --author 'Claude Code' 'Fixed the null check'
```

In `--json` mode, set the `file` field on the entry. Review-level IDs (`r_…`) are globally unique and never need this.
</important>

<important if="you are responding to plan-mode comments (review file under ~/.crit/plans/)">
Plan reviews (via `crit plan` or the ExitPlanMode hook) store the review file in `~/.crit/plans/<slug>/`. **Always pass `--plan <slug>`** — without it, `crit comment` looks in the project root and won't find the comments. The slug is shown in the review feedback prompt.

```bash
crit comment --plan my-plan-2026-03-23 --reply-to c_a1b2c3 --author 'Claude Code' 'Updated the plan'
```
</important>

<important if="a daemon is running and you need to edit, delete, or re-anchor an existing comment">

While `crit` is running, the daemon holds the comments in memory and is the
source of truth. Within a round it merges only structural changes from the
review file, so a body you edit directly in `review.json` reaches neither the
daemon nor the browser, and the daemon can overwrite it on its next write. The
next round transition reloads file comments from disk and would carry that body
forward, which makes the outcome depend on timing you do not control. There is
no CLI verb for editing a comment body — use the HTTP API.

Get the port from `crit status --json` (`.daemon.port`), and poll
`GET /api/session` until it answers 200 before calling anything else. 503 means
the session is still initialising; 500 means initialisation failed, and no
amount of waiting fixes it.

```
GET|POST   /api/file/comments?path=<repo-rel>              list / add line and file comments
PUT|DELETE /api/comment/<id>?path=<repo-rel>               update body / delete
POST       /api/comment/<id>/replies?path=<repo-rel>       add a reply
PUT|DELETE /api/comment/<id>/replies/<rid>?path=<repo-rel> edit / delete a reply
PUT        /api/comment/<id>/resolve?path=<repo-rel>       set resolved
GET|POST   /api/comments                                   list / add review-level comments
PUT|DELETE /api/review-comment/<id>                        review-level update / delete
POST       /api/review-comment/<id>/replies                add a review-level reply
PUT|DELETE /api/review-comment/<id>/replies/<rid>          edit / delete one
PUT        /api/review-comment/<id>/resolve                set resolved
GET        /api/health                                     liveness, plus browser_clients
```

- `POST /api/file/comments` takes `{start_line, end_line, body, author}` and
  stamps the session's focus onto the new comment, which makes it the reliable
  authoring path inside a `--pr` or `--range` session. A file-level comment
  needs `"scope": "file"`; without it the omitted lines fail the line-range
  check.
- `PUT` replaces the body and leaves a line comment's range alone; to move one,
  POST a copy at the new lines and DELETE the old id. It does carry
  `dom_anchor`, so a live-mode pin can be re-anchored in place.
- Review-scope ids (`r_…`) live under `/api/review-comment/<id>`, take no
  `path`, and are listed by `GET /api/comments`.
- A round transition mints fresh ids for every carried-forward comment on a
  file, line scope and file scope alike, while review-level ids stay put.
  Re-read ids from `GET /api/file/comments` before editing one.

</important>

<important if="you are syncing with a GitHub PR or GitLab MR (pull or push)">

```bash
crit pull [number|url]                                   # Fetch PR/MR review comments into the review file
crit push [--dry-run] [--event <type>] [-m <msg>] [n]    # Post review comments to a PR/MR
crit pull --forge gitlab 42                              # Force GitLab when auto-detect is ambiguous
```

Requires `gh` (GitHub) or `glab` (GitLab) installed and authenticated. Change number is auto-detected from the current branch when possible. Set `"forge"` / `"gitlab_url"` in config for self-managed hosts, or pass `--forge`.

`--event` values: `comment` (default), `approve`, `request-changes`. `-m` adds a review-level body message.
</important>

<important if="the user asked to share, get a URL, get a QR code, or unpublish a review">

```bash
crit share <file> [file...]                          # Upload and print URL
crit share --qr <file>                               # Also print QR code (terminal only)
crit share --org <slug> <file>                       # Share under an organization
crit share --org <slug> --visibility unlisted <file> # Org share with explicit visibility
crit unpublish [file...]                              # Remove shared review
```

- **No server needed** — reads files directly from disk. If a review file exists, comments for the shared files are included automatically.
- **Always relay the output** — copy the URL (and QR if used) into your response. Don't make the user dig through tool output.
- **`--qr` is terminal-only** — skip in mobile apps, web chat UIs, or anywhere Unicode block characters won't render correctly.
- **`--org <slug>`** shares under an organization. Visibility defaults to `organization` (members only). Override with `--visibility` (`organization`, `unlisted`, `public`).
- **Unpublish uses the persisted delete token** in the review file — no extra args needed.
</important>
