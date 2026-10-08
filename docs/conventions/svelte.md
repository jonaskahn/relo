# Svelte and TypeScript conventions

Stack: Svelte 5 (runes) · SvelteKit 2 · TypeScript (strict) · Tailwind CSS v4.
Principles: Clean Code · KISS · DRY · YAGNI · SOLID.

Read [CONTRIBUTING.md](../../CONTRIBUTING.md) and the
[principles](../principles.md) for shared principles, and
[PRODUCT.md](../design/PRODUCT.md) and [DESIGN.md](../design/DESIGN.md) before
planning or writing console UI code. This guide applies to `console/`.

Theme tokens live in `console/src/app.css`; a component reads them instead of
inventing its own color. Status always pairs color with an icon or a word.

Rules are mandatory unless marked *(prefer)*.

---

## 1. Architecture

### 1.1 Static SPA — no server layer

The console is a SvelteKit 2 single-page app built with `adapter-static`
(`fallback: 'index.html'`). The Go daemon embeds the built files and serves
them. There is no SvelteKit server, no SSR, no `hooks.server.ts`, no
`+page.server.ts`, no actions, no remote functions — the daemon's HTTP API is
the only backend.

Absent SvelteKit files and why:

| File                                                                      | Reason absent                                                                                     |
|---------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------|
| `src/lib/server/`, `src/hooks.server.js`, `src/instrumentation.server.js` | Need an adapter that runs a server                                                                |
| `src/params/`                                                             | No route with a dynamic segment yet                                                               |
| `src/service-worker.js`                                                   | Go already serves the static build                                                                |
| `src/error.html`                                                          | No render path under `adapter-static` with `ssr = false`; `+error.svelte` handles in-app failures |
| `tests/` (Playwright)                                                     | Tests run through Vitest in `src`                                                                 |

Add a file from that list only when the architecture it serves arrives.

### 1.2 Structure

```text
src/
├─ app.css               # Tailwind v4 tokens and globals
├─ app.d.ts              # App types
├─ app.html              # shell
├─ lib/
│  ├─ api.ts             # typed fetch client (CSRF, Accept-Language, ApiError)
│  ├─ sse.ts             # SSE live-update stream
│  ├─ types.ts           # shared domain types
│  ├─ *.ts               # pure view logic modules
│  ├─ *.svelte.ts        # shared reactive state (console-state, session, theme …)
│  ├─ *.test.ts          # colocated Vitest tests
│  ├─ i18n/locales/      # one JSON per locale
│  └─ components/
│     ├─ ui/             # shared UI: shadcn-svelte, Bits UI wrappers, app components
│     └─ <area>/         # feature-area components
└─ routes/               # pages and redirect stubs
```

Feature areas under `lib/components/` are named for the page or surface they
serve — `agents/`, `chat/`, `dashboard/`, `error/`, `groups/`, `keys/`,
`logs/`, `providers/`, `settings/`. A page that starts collecting its own
components creates its area folder rather than growing the route file past
what a reader can hold.

### 1.3 Placement rules

1. Pages live in `src/routes`. Redirect-only `+page.ts` files preserve old URLs.
2. Shared UI lives in `src/lib/components/ui/`. Feature components live in area
   folders under `src/lib/components/`.
3. Every component file is named in kebab-case after the component it
   exports (`CenteredModal` → `centered-modal.svelte`); the imported
   identifier stays PascalCase. `ui/` is one flat file per component. A
   feature area may nest one folder per surface when a modal grows its own
   sections (`providers/add/`).
4. Pure view logic lives in `src/lib/*.ts`, beside colocated `*.test.ts`.
5. Shared reactive state lives in `*.svelte.ts` modules.
6. Import via `$lib/...`. No barrel files (`index.ts` re-exports) — import
   from the defining module.
7. No new npm dependency without stating the concrete need.

---

## 2. Data flow

| Need                               | Use                                         |
|------------------------------------|---------------------------------------------|
| API calls to the daemon            | `src/lib/api.ts` client                     |
| Live updates                       | `src/lib/sse.ts` stream                     |
| Component-local state              | `$state` in the component                   |
| Shared client state                | class with `$state` fields in `*.svelte.ts` |
| Shareable UI state (filters, sort) | URL search params                           |

1. All data comes from the Go daemon via `api.ts` or `sse.ts`. There is no
   server `load`, no actions, no remote functions.
2. Start independent requests together. No fetch waterfalls.
3. Component `await` is experimental and not enabled in `svelte.config.js`.
   Keep API loading consistent with neighboring pages. If async components are
   adopted later, pair `await` with `<svelte:boundary>` for pending and error
   states.

---

## 3. Svelte 5 components

### 3.1 Syntax (legacy is forbidden)

| Forbidden                     | Required                                                  |
|-------------------------------|-----------------------------------------------------------|
| `export let x`                | `let { x } = $props()`                                    |
| `$: y = x * 2`                | `let y = $derived(x * 2)`                                 |
| `on:click`                    | `onclick`                                                 |
| `<slot>`, `$$slots`           | `{#snippet}` + `{@render}`                                |
| `createEventDispatcher`       | callback props                                            |
| `use:action`                  | `{@attach ...}` (existing actions may stay until changed) |
| `<svelte:component this={C}>` | `<C />`                                                   |

### 3.2 Props

1. Every component declares `interface Props` and destructures `$props()`.
2. Events are callback props named `on<verb>` (lowercase).
3. Required props have no default. Optional props have a default in the
   destructure.
4. Use one union prop instead of several booleans.
5. Pass the narrowest data needed (`{ name, avatarUrl }`, not the whole entity).
6. Wrapper components spread rest attributes onto the root element and type
   them with `svelte/elements`.
7. `$bindable` only for form-control semantics (`value`, `open`, `checked`).
   Otherwise one-way data + callbacks.
8. Anything computed from a prop is `$derived`.

### 3.3 Composition

1. `children` for main content; named snippet props for regions; snippet
   parameters instead of slot props.
2. Optional snippets: `{@render footer?.()}`.
3. Extend behavior with snippets and callbacks, not boolean flags.
4. Prop drilling max 2 levels. Beyond that, compose (pass a snippet) first,
   then use context.
5. Extract a shared component only after 3 real uses.
6. No config-driven mega-components. No component inheritance.

### 3.4 Size and responsibility

1. One responsibility per component. Review for a split above ~150 lines,
   ~7 props, or 2 levels of nested `{#if}`/`{#each}`.
2. `lib/components/ui/*` components are presentational: props in, callbacks out.
3. Template expressions stay trivial: property access, simple ternary, or a
   function call. Move logic to `$derived` or a `*.ts` module.
4. Every `{#each}` has a stable unique key (never the index).

---

## 4. Reactivity (runes)

1. `$state` only for values that must trigger updates. Everything else is a
   plain `const`/`let`.
2. Compute with `$derived` / `$derived.by`. Never assign state inside `$effect`
   to mirror other state.
3. `$state.raw` for large data that is replaced, not mutated.
4. `$effect` is a last resort. First try: event handler, function binding,
   `{@attach}`, `<svelte:window>`.
5. Every `$effect` is idempotent and returns cleanup when it allocates resources.
6. Runes are allowed only in `.svelte` and `.svelte.ts` files. Never in pure
   `*.ts` modules.

### Shared state

1. Shared reactive state is a class in `*.svelte.ts` with `$state` fields (e.g. `console-state.svelte.ts`,
   `session.svelte.ts`, `theme.svelte.ts`).
2. No module-level mutable state holding user or request data.
3. Pass reactive values across function boundaries as getters or objects, not
   destructured primitives.

---

## 5. TypeScript

### 5.1 Type safety

1. `any` is forbidden. Use `unknown` and narrow.
2. `as` only for `as const`. Non-null `!` only right after a checked invariant.
3. Use `import type` for type-only imports.
4. No `enum`. Use `as const` arrays/objects and derive union types.
5. Prefer `readonly` on data types and `readonly T[]` for function inputs that
   must not be mutated.

### 5.2 Naming

1. `PascalCase` types and components; `camelCase` values and functions;
   booleans read as predicates (`isOpen`, `hasError`).
2. No `I` prefix on interfaces.
3. Intent-revealing names. If a name needs a comment to explain what it holds,
   rename it.

### 5.3 Self-describing code and comments

Code must read as prose. Names, structure, and small functions carry the
meaning — not comments.

**Exported functions and types** get a JSDoc comment that says what the symbol
does for its caller. The comment describes the contract, not the
implementation. It does not restate the signature.

```ts
/** Returns the human-readable label for an account, falling back to the
 *  provider name when no alias is set. */
export function accountLabel(account: Account): string { … }
```

**Unexported / internal functions** do not get a comment. Rename, simplify,
or split them until the code is descriptive.

```ts
// Banned — the name already says this.
// Filters the active connections.
function filterActive(connections: Connection[]): Connection[] { … }
```

```ts
function activeConnections(connections: Connection[]): Connection[] { … }
```

**Inline comments** are allowed only when they explain **why**: a business
rule, a workaround for a known upstream bug, or a safety invariant that the
code cannot show on its own. Place them on the block they explain. Do not
narrate what the next line does.

These comments are always forbidden:

| Pattern              | Example                     | Why it is banned                               |
|----------------------|-----------------------------|------------------------------------------------|
| Phase / task markers | `// Phase 2`, `// Task-027` | Couples code to project artifacts that rot.    |
| Tutorial comments    | `// iterate over the array` | Restates what the code already says.           |
| "What" narration     | `// check if user is admin` | Rename the function or variable instead.       |
| Commented-out code   | `// cfg.legacy = true`      | Use version control. Dead code is deleted.     |
| Section dividers     | `// ---- helpers ----`      | Extract a separate file if grouping is needed. |

Before writing a comment, ask: can I rename something, extract a function, or
simplify the logic so the comment becomes unnecessary? The exception is the
reason behind a rule the code cannot express:

```ts
// Tombstoned accounts keep their usage rows so a re-added account
// with the same email never inherits the old one's history.
if (account.deletedAt) continue;
```

### 5.4 Functions

1. One responsibility per function. If you describe what it does and use the
   word "and," it does two things — split it.
2. Maximum 3 positional parameters. Beyond that, use an options object with
   named fields.
3. Keep functions short. If a function needs a section divider, it needs a
   split.

---

## 6. Principles → concrete rules

| Principle      | Rule                                                                                                                                            |
|----------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| **Clean Code** | Intent-revealing names. One level of abstraction per function. ≤ 3 params. Comment *why*, not *what*. No commented-out code.                    |
| **KISS**       | Use the platform first: semantic HTML, URL params for UI state, plain variables for non-reactive values. Reach for `$derived` before `$effect`. |
| **DRY**        | Deduplicate knowledge (types, rules, transforms), not keystrokes. Reuse markup with snippets. Extract after 3 uses.                             |
| **YAGNI**      | No speculative abstractions, generic components, or flexibility that wasn't requested. Add structure when there is real logic or a test need.   |
| **SRP**        | Component renders; `*.ts` module decides; `api.ts` does I/O.                                                                                    |
| **OCP**        | Extend with snippets and callback props, not boolean flags.                                                                                     |
| **LSP**        | Wrappers honor the wrapped element's contract (rest attrs, native events).                                                                      |
| **ISP**        | Small `Props`; narrow function signatures (one concern's needs).                                                                                |
| **DIP**        | Components depend on `*.ts` modules for logic, not on each other's internals.                                                                   |

---

## 7. UI and copy

Reuse the shadcn-svelte style and Bits UI wrappers in
`src/lib/components/ui/`. Use `tailwind-variants` and the existing `cn` helper
for component variants. Read color, type, radius, and spacing tokens from
`src/app.css` (Tailwind CSS v4). Use the vendored Tabler paths in
`Icon.svelte`, `svelte-sonner` for toasts, `CenteredModal` for ordinary
overlays, and `ConfirmDialog` or AlertDialog for destructive confirmations.
Status must have a word or icon as well as a color.

Operator-facing copy belongs in `src/lib/i18n/locales/<tag>.json` and is
rendered through svelte-i18n. Add each new key to every shipped locale in the
same change; the parity tests fail when a catalog drifts.

Semantics and accessibility:

1. Use semantic elements (`<button>`, `<a href>`, `<label for>`, `<form>`).
   No `onclick` on `<div>`.
2. Every `{#each}` keyed by stable id.

---

## 8. Testing

| Layer                            | Tool          | What                          |
|----------------------------------|---------------|-------------------------------|
| Pure view logic (`*.ts`)         | Vitest (node) | rules, transforms, formatters |
| Reactive modules (`*.svelte.ts`) | Vitest        | store logic                   |
| Components                       | Vitest        | behavior and error states     |

1. Test visible behavior and error states rather than duplicating component
   internals.
2. `src/lib/ui-consistency.test.ts` enforces shared UI constraints.
3. No whole-DOM snapshots.

---

## Verification

Run `make ci-console` for console changes. It runs `svelte-check`, ESLint,
Prettier in check mode, Vitest, and the production build.

---

## 10. Anti-patterns (reject on sight)

1. `$effect` that writes state derived from other state → `$derived`.
2. `$effect` reacting to a click or input → event handler.
3. Fat component (fetch + validate + compute + render) → split by
   responsibility.
4. Boolean-flag props → union variants + snippets.
5. Prop drilling beyond 2 levels → snippets, then context.
6. `any`, or `as Foo` on unvalidated input → narrow or validate.
7. `{#each}` keyed by index → stable id.
8. Legacy syntax (`export let`, `$:`, `on:`, `<slot>`) → Svelte 5
   equivalents (§3.1).
9. Barrel `index.ts` files → direct imports.
10. `enum` → union literal + `as const`.
11. Adding `+page.server.ts`, actions, hooks, or remote functions → no server
    in this architecture.

---

## 11. Sources

- [Svelte 5 runes, snippets, attachments, and async templates](https://svelte.dev/docs/svelte/overview)
- [SvelteKit documentation](https://svelte.dev/docs/kit/introduction)
  and [SvelteKit 3 release candidate](https://next.svelte.dev/blog/sveltekit-3-release-candidate)
- [SvelteKit remote functions](https://svelte.dev/docs/kit/remote-functions)
