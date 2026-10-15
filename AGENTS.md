# AGENTS.md

Guidelines for AI coding agents working in this repository.

**The best code is the code that was never written.**

## Project context

Before planning a change, read [CONTRIBUTING.md](CONTRIBUTING.md) and the docs
its "Before you write code" table lists for the area you are touching.

## Commands

- Go work: `make ci-backend`
- Console work: `make ci-console`
- Work touching both: run both targets; `make ci` is the full pre-push gate

## Behavioral guidelines

These favor caution over speed: be lazy about writing code, not about
verifying it. For trivial tasks, use judgment.

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

- State your assumptions explicitly.
- When a request has more than one reasonable reading, or something is
  unclear, name it and ask instead of picking silently.
- If a simpler approach exists, say so, and push back when warranted.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

Before writing any code, walk this ladder and stop at the first rung that
holds:

1. **YAGNI:** Build only what was asked: no speculative features,
   flexibility, or configuration.
2. **Reuse:** Does a helper, type, or pattern already exist in this codebase?
   Reuse it.
3. **Stdlib:** Does the standard library do this natively?
4. **Native platform:** Is there a built-in platform, browser, or DB feature
   that avoids code?
5. **Installed dependency:** Can an already-installed package solve this? Do
   not add new ones.
6. **One-liner:** Can this be achieved securely in one line?
7. **Minimum working code:** Build the bare minimum that works safely.

Also:

- No abstractions for single-use code.
- No error handling for impossible scenarios. Never skip validation at trust
  boundaries, auth checks, or real failure paths.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Where the conventions docs are silent, match the surrounding code's style,
  even if you'd do it differently.
- Remove imports, variables, and functions your change made unused. Mention
  unrelated dead code; leave it in place unless asked.

Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

- "Add validation" → "Write tests for invalid inputs, then make them pass."
- "Fix the bug" → "Write a test that reproduces it, then make it pass."
- "Refactor X" → "Ensure tests pass before and after."

Strong success criteria let you loop independently; weak ones ("make it work")
need constant clarification.

## Final report

When you finish, report what changed, how you verified it (quote the output of
any check that failed), and anything you deliberately skipped with the exact
condition under which it should be added. Keep it to what the reader needs to
act on.

## Deliberate simplifications

If you deliberately simplify a complex problem (naive loop, basic lock, etc.),
leave one short comment in the file's own comment syntax stating the constraint
and the upgrade path.
