# AGENTS.md

Guidelines for AI coding agents working in this repository.

**The best code is the code that was never written.**

## Project context

- Read [CONTRIBUTING.md](CONTRIBUTING.md) and the [docs index](docs/README.md)
  (principles, architecture, glossary, API, product rules) before planning or
  writing any change.
- Read [Go conventions](docs/conventions/golang.md) before planning or writing
  Go code under `daemon/`.
- Read [Svelte conventions](docs/conventions/svelte.md) before planning or
  writing code under `console/`.
- Before console/desktop UI work involving pages, components, styles, copy, or
  motion, also read [PRODUCT.md](docs/design/PRODUCT.md) and
  [DESIGN.md](docs/design/DESIGN.md).

## Commands

- Go work: `make ci-backend`
- Console work: `make ci-console`
- Cross-cutting work: `make ci` (when practical)

## Behavioral guidelines

Combine these with the project context and commands above. They favor caution
over speed: be lazy about writing code, not about verifying it. For trivial
tasks, use judgment.

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them; don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

Before writing any code, walk this ladder and stop at the first rung that
holds:

1. **YAGNI:** Does this speculative feature need to exist? If not, skip it.
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

- No features beyond what was asked.
- No abstractions for single-use code.
- No flexibility or configurability that wasn't requested.
- No error handling for impossible scenarios. Never skip validation at trust
  boundaries, auth checks, or real failure paths.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes,
simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it; don't delete it.
- Remove imports, variables, and functions that YOUR changes made unused; leave
  pre-existing dead code unless asked.

Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

- "Add validation" → "Write tests for invalid inputs, then make them pass."
- "Fix the bug" → "Write a test that reproduces it, then make it pass."
- "Refactor X" → "Ensure tests pass before and after."

For multi-step tasks, state a brief plan:

```text
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently; weak ones ("make it work")
need constant clarification.

## Response format

- Functional code first.
- Then at most 3 short bullets: what was skipped, and the exact condition under
  which it should be added.
- No unrequested prose, conversational filler, or architectural essays.
- Exceptions: clarifying questions (guideline 1) and the brief plan for
  multi-step tasks (guideline 4) may come before the code.

## Deliberate simplifications

If you deliberately simplify a complex problem (naive loop, basic lock, etc.),
leave one short comment in the file's own comment syntax stating the constraint
and the upgrade path.
