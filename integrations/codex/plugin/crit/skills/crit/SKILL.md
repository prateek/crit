---
name: crit
description: "Review code changes, a plan, a live page (running dev server), or a local HTML file with Crit inline comments and structured human feedback. Use only when the user explicitly invokes /crit or directly asks to use Crit; a generic review request does not count."
---

# Review with Crit

This is the interactive, human-in-the-browser review cycle. Run it only after
the user explicitly invokes `/crit` or directly asks to use Crit. A generic
request to review code, a plan, a diff, a PR, or a page does not count.

Review and revise code changes, plans, live pages (running dev servers, staging URLs), or local HTML files using `crit` for inline comment review.

## Step 1: Pass arguments to `crit`

The CLI auto-detects the review mode from its arguments. **Do not ask the user which mode to use.** Pass arguments through:

```
crit <arguments>               # file, dir, URL, .html — CLI auto-detects mode
crit --pr <num|url>            # GitHub PR (range mode)
crit --mr <iid|url>            # GitLab MR (range mode)
crit --range <base>..<head>    # commit range (range mode)
crit                           # no args → branch diff
```
When there is no argument, or the argument is a bare word that names no file
(`/crit`, `crit this`) and is not a flag or a subcommand the CLI understands
(`live`, `preview`, `plan`, `story`), check conversation context, in order:

1. A plan file was written earlier in this conversation → `crit <plan-file>`
2. The user named the artifact ("the table", "that write-up", "the plan") →
   review that artifact
3. The previous assistant message is substantial prose the user is plausibly
   reacting to, a plan or an analysis or a set of options → ask which they
   meant, in one line: the code changes, or that message
4. Otherwise → bare `crit` (branch diff)

Asking which **target** to review is correct when it is genuinely ambiguous.
That differs from asking which **mode** to use, which the CLI reads off the
argument. Launching blocks on a browser round trip, so a wrong target costs the
user more than one short question.

To review something that exists only in the conversation, write it to a
markdown file and pass that path. `crit` reviews any file, including one
outside a repository.

<important if="the branch sits in a stack of dependent branches">

Range mode is what makes crit stack-aware. Bare `crit` and `--base-branch` both
review in working-tree focus, where the picker offers no stack switcher.
`--range` puts the session in range focus, where the picker offers the layers it
can see and the reviewer hops between them.

```bash
crit --pr <num>                      # one layer, exactly what that pull request holds
crit --mr <iid>                      # the same, on GitLab
crit --range <parent-branch>..HEAD   # one layer, while the parent is an ancestor of HEAD
crit --range <trunk>..HEAD           # every layer, the stack as one diff
```

Prefer `--pr`, or `--mr` on GitLab, for a single layer that has one. `--range`
is a two-dot diff between the two tips, so once the parent branch advances it
pulls in commits that belong to the parent. Both forge modes diff from the
merge-base instead and do not, as long as the base and head commits resolve
locally: under `--remote` the merge-base lookup can fail and crit falls back to
the base tip, which is a two-dot diff again.

The picker walks at most twenty ancestor commits and, on git, keeps only the
topic chain, so a deep stack or a branch that is not an ancestor of HEAD will be
missing from the switcher.

`crit story` resolves its own session from its own arguments rather than
inheriting an open review's focus. Repeat whichever of `--pr`, `--mr` or
`--range` you used, or the story narrates the working tree instead of the
commits.
</important>

<important if="the user wants to open the review from another device — e.g. a phone over Tailscale">
Keep crit on loopback and reverse-proxy in. Same Step 1 args still apply (file, bare `crit`, etc.):

```bash
crit --public-url "https://<machine>.ts.net" --allow-unauthenticated-network --no-open [args…]
# then: tailscale serve --bg --https=443 http://127.0.0.1:<port>
```

- `--public-url` only changes the URL crit prints — it does **not** expose the server.
- `--allow-unauthenticated-network` is required with `--public-url` (even on loopback) and with any non-loopback `--host`. Crit has no auth: anyone who can reach the URL can read the repo and post comments that may trigger agents. Confirm the user wants that blast radius.
- **Do not bind `--host` to a Tailscale/LAN IP** — proxy with `tailscale serve` (or an SSH tunnel). Get `<port>` from crit's startup output, or fix it with `-p`. Relay the URL crit prints (the public one), not localhost.
</important>

## Step 2: Launch crit and wait for review completion

**CRITICAL — you MUST run this step. Do NOT skip it. Do NOT proceed without it.**

Run `crit` and wait until it exits:

```bash
crit <plan-file>   # specific file
crit               # git mode
```

If a crit server is already running from earlier in this conversation, `crit` automatically connects to it. Starting from scratch, it spawns the daemon, opens the browser, and blocks until the user clicks "Finish Review".

`crit` prints the review URL on startup (e.g. `Started crit daemon at http://localhost:<port>`). Relay it verbatim:

> **"Crit is open at http://localhost:<port>. Leave inline comments, then click Finish Review."**

**Do NOT proceed until `crit` completes.** Do NOT ask the user to type anything. Do NOT read the review file early. Wait for the command to finish — that is how you know the human is done reviewing.

While `crit` is still running, do not send user-facing progress updates. Treat the wait loop as internal state.

## Step 3: Read the review output

When `crit` completes, read **stdout** and follow its instructions. Check **stderr** for `approved: true` or `approved: false`.

When a comment has `quote`, `anchor`, or `drifted`:
- `quote`: the specific text the reviewer selected — focus your changes on the quoted text rather than the entire line range
- `anchor`: use it to locate the current position of the content; line numbers may be stale after edits
- `drifted: true`: original content was removed or heavily rewritten — line numbers are approximate at best

**Fallback** (mid-round re-entry, plan hooks, or headless workflows): `crit comments` / `crit comments --json`. Use `crit comments --plan <slug>` for plan-mode reviews.

## Step 4: Address each review comment

For each unresolved comment:

1. Understand what the comment asks for
2. If it contains a suggestion block, apply that specific change
3. Revise the referenced file (plan or code file from the diff)
4. Reply with what you did: `crit comment --reply-to <id> --author 'Codex' '<what you did>'` (reply bodies support markdown)
5. **Do not pass `--resolve`.** Resolving is the reviewer's call. Only add `--resolve` if the user explicitly asks.

Editing the plan file triggers Crit's live reload — the user sees changes in the browser immediately.

<important if="you are revising in plan mode">
Re-emit the revised plan inside `<proposed_plan>...</proposed_plan>` so Crit can review the new version.
</important>

### When replying to multiple comments

Use `--json` for a single bulk call instead of one invocation per comment:

```bash
echo '[
  {"reply_to": "c_a1b2c3", "body": "Fixed"},
  {"reply_to": "c_d4e5f6", "body": "Refactored as suggested"}
]' | crit comment --json --author 'Codex'
```

## Step 5: Signal completion and start next round

**CRITICAL — you MUST run this step. Do NOT skip it. Do NOT proceed without it.**

The finish prompt on stdout includes the command to run again — use it to start a new round.

On subsequent calls, `crit` automatically signals round-complete first, then blocks until the next "Finish Review" click.

Tell the user: **"Changes applied. Review the diff in your browser and click Finish Review when ready."**

**Do NOT proceed until the background `crit` task completes.** When it does, return to Step 3. If the user finishes with zero comments, the review is approved — stop the loop and proceed.

## Sharing

If the user asks for a URL, a shareable link, or to share the review:

```bash
crit share <file>
```

**Always relay the full output to the user** — copy the URL directly into your response. Don't make them dig through tool output.

To remove a shared review:

```bash
crit unpublish [file...]
```

### QR codes

Only use `--qr` in real terminal environments with monospace rendering. Skip it in mobile apps or web chat UIs — Unicode block characters won't render.

```bash
crit share --qr <file>
```
