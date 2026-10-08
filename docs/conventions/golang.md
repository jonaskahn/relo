# Go conventions

Read [CONTRIBUTING.md](../../CONTRIBUTING.md) for the workflow, plus the
[principles](../principles.md), [architecture](../architecture.md), and
[glossary](../glossary.md). These rules apply when planning
or writing Go code in `daemon/`.

## The Cardinal Rule: Self-Describing Code

Code must read as prose. Names, structure, and small functions carry the
meaning — not comments.

Comments exist for two purposes: **godoc on exported symbols and files**, and
a short note on logic whose reason is not visible from the code.

### Exposed API

Every exported/exposed type, function, and variable has a godoc comment. The comment
says what the symbol does for its caller and why it exists in business terms.
It does not restate the signature.

```go
// AccountStore manages the lifecycle of provider accounts,
// including credential rotation and tombstone retention.
type AccountStore struct { ... }

// Resolve returns the active API key for the given provider,
// expanding environment variable and keychain references.
func (ks *KeyStore) Resolve(provider string) (string, error) { ... }
```

A const or var group may share one comment when the names are one idea. Give
a member its own comment only when its meaning is not the group's.

### Files

Every non-test Go file opens with a comment. The first file of a package
carries the package comment. Every other file carries a file comment that
names what that file owns, not a list of the functions inside it.

```go
// Package account owns one credential in a connection's pool:
// how it is stored, rotated, and retired.
package account
```

```go
// Rotation applies a new credential and retires the one it replaces.
package account
```

### Unexported code

Do not comment an unexported function, type, or variable. Rename, clean, or
split it until the code is descriptive.

```go
// Banned — the name already says this.
// validateAdmin checks the admin listener.
func validateAdmin(cfg Config) error { ... }
```

```go
func validateAdminListener(cfg Config) error { ... }
```

### Logic worth a comment

A comment inside a function is allowed only when it explains **why**: a
business rule, a workaround for a known upstream bug, or a safety invariant
that the code cannot show on its own. Place it on the block it explains.
Do not narrate what the next line does.

These comments are always forbidden:

| Pattern              | Example                            | Why it is banned                                                   |
|----------------------|------------------------------------|--------------------------------------------------------------------|
| Phase / task markers | `// Phase 2`, `// Task-027`        | Couples code to project management artifacts that rot immediately. |
| Tutorial comments    | `// iterate over the range 0 to 9` | Restates what the code already says.                               |
| "What" narration     | `// check if user is admin`        | If you need this comment, rename the function or variable instead. |
| Commented-out code   | `// cfg.Legacy = true`             | Use version control. Dead code is deleted, not hidden.             |
| Section dividers     | `// ---- helpers ----`             | Extract a separate file or package if grouping is needed.          |

Before writing a comment, try to rename something, extract a function, or
simplify the logic so it becomes unnecessary. The exception is the reason
behind a rule the code cannot express:

```go
// Tombstones are retained so a deleted account's usage rows
// are never re-attributed to a re-added account with the same email.
if account.DeletedAt != nil {
    continue
}
```

## Naming

### Functions

A function's name reflects its scope. The higher the abstraction, the shorter
and broader the name. As you descend the call tree, names become more specific.

```go
// Top level — broad name.
config, err := configuration.Parse(path)

// One layer deeper — slightly more specific.
func Parse(filepath string) (Config, error) {
    switch fileExtension(filepath) {
    case "json":
        return parseJSON(filepath)
    case "toml":
        return parseTOML(filepath)
    }
}

// Leaf — the most specific.
func fileExtension(filepath string) string {
    segments := strings.Split(filepath, ".")
    return segments[len(segments)-1]
}
```

Never front-load every detail into the name:
`DetermineFileExtensionAndParseConfigurationFile` is harder to read than a
short call chain of well-named functions.

### Variables

The opposite of functions: variables start specific at wide scope and become
shorter as scope narrows.

```go
func BrandsToBeerList(beerBrands []BeerBrand) []Beer {
    var beers []Beer
    for _, brand := range beerBrands {
        for _, b := range brand {
            beers = append(beers, b)
        }
    }
    return beers
}
```

Single-letter variables are fine inside a tight loop body or a 3-line closure.
Beyond that, spell it out. Never name a variable after its type (`userStruct`, `itemMap`).

### Errors

Sentinel errors are package-level variables prefixed with `Err`:

```go
var (
    ErrItemNotFound      = errors.New("item not found in store")
    ErrInsufficientQuota = errors.New("account quota exhausted")
)
```

Every known error condition has such a variable. Never return a raw
`errors.New("...")` from inside a function. Callers branch with `errors.Is`
against the variable, never by string comparison.

## Functions

**Target length: 5–8 lines.** A function does one thing. If you describe what
it does and use the word "and," it does two things — split it.

**Maximum parameters: 2–3.** Beyond that, introduce an options struct with
named fields so callers never pass five positional booleans.

**Early returns.** Push error paths left. Never nest `if/else` deeper than
two levels.

If the `value, err :=` pattern appears more than once, that is a signal to
extract a helper.

## Variable Scope and Mutability

Declare variables as close to their usage as possible. Prefer `:=` at the
point of first use over a bare `var` declaration five lines earlier.

Minimize mutable scope. When a value is computed, return it from a function
rather than mutating a variable declared in an outer scope. If a struct field
must be protected from external mutation, make it private and expose a
getter.

No package-level mutable variables unless wrapped in a struct with controlled
access. Mutable globals make state unpredictable and break test isolation.

## SOLID

**Single Responsibility.** Each package owns one concern; each function does
one task. The Go module lives in `daemon/`. In `daemon/internal/`,
`adapters/sqlite/` owns the database, `adapters/secrets/` the keychain, and
`adapters/wire/` wire translation; each feature package (`catalog/`,
`account/`, `routing/`, `activity/`, `access/`) owns the rules of one part of
the product.

**Open/Closed.** Extend behavior through new interface implementations, not
by modifying existing functions. Adding a new provider means writing a new
adapter file, not editing a switch statement in the router.

**Liskov Substitution.** Any implementation of an interface must be safely
substitutable. Verify at compile time:

```go
var _ io.Writer = &NullWriter{}
var _ Adapter   = &AnthropicAdapter{}
```

**Interface Segregation.** Interfaces stay small — one to three methods. A
consumer that only reads should accept a `Reader`, not a `ReadWriteCloser`.

**Dependency Inversion.** High-level packages depend on interfaces defined
in their own package or in a shared contract package. Concrete
implementations are injected through constructors, so the caller decides
whether to inject the production keychain, a test stub, or a mock:

```go
func NewServer(cfg *config.Config, db Storage, secrets SecretStore) *Server
```

## Clean Go Specifics

**Return default null values.** Define a `NullItem`, `EmptyConfig`, etc.
with safe zero-initialized fields so callers never dereference a nil
property on a bare `Type{}`.

**Pointer discipline.** Accept values when possible. Accept pointers only
when mutation is the explicit purpose of the function. Never return a
pointer to an internal store entry — return a copy so callers cannot corrupt
shared state.

**Hide `any` / `interface{}`.** When a dependency forces you to use the
empty interface, wrap it in a typed accessor so the rest of the codebase
never touches `interface{}` directly.

**Nil-safe constructors.** Every struct with interface, map, slice, or
channel fields must have a constructor that initializes them. Bare `Type{}`
with nil internals is a panic waiting to happen.

**Closures over switch bloat.** When the set of operations is open-ended,
accept a closure rather than growing a switch statement with every new
operation.

## Error Handling

All errors are returned, never swallowed. `os.Exit` and `log.Fatal` appear
only in `daemon/cmd/relo/main.go`. Every other package returns an error to
its caller.

Wrap errors with context using `%w` so the chain is inspectable with
`errors.Is` and `errors.As`:

```go
account, err := store.GetAccount(id)
if err != nil {
    return fmt.Errorf("resolve account for routing: %w", err)
}
```

The wrapping message describes the action that failed from the caller's
perspective, not the callee's internals.

For conditions that carry dynamic detail, use a typed error struct that
implements the `error` interface and exposes a `Type()` method for safe
comparison.

## Testing

**Table-driven tests** for functions with multiple input/output cases.

**Test names describe the scenario**, not the function ordinal.
`TestGetItem_NotFound` over `TestGetItem2`.

**Mock via interfaces.** Every external dependency (database, keychain, HTTP
upstream) is accessed through an interface. Tests inject a stub or mock, never
a live service.

**No mirror tests.** A test that duplicates the implementation line-for-line
catches nothing and breaks on every refactor. Test the observable behavior:
given this input, assert that output.

### Test Layout

**White-box tests live beside the package they measure**: `sse_test.go` in
`adapters/wire/sse/` tests that package's own helpers. **Black-box, integration,
and end-to-end tests live under `daemon/tests/`**, and they reach a package
the way the product does: through its exported surface, over HTTP, against a
mock upstream. Fixtures and stores come from `tests/testkit`.

`daemon/tests/coverage.tsv` maps each internal package to the test packages
that measure it, so the gate can tell a package that is unmeasured from one
that is measured and weak. A row may name its own floor when the package
stands below the 90 percent target; the number then records where it actually
stands and the gate fails on any regression. Raising a floor is the work.

`make cover` fails when a mapped test package is missing, when a source
package is neither mapped nor declared exempt, and when a package measures
below its floor.

## Tooling

Run `gofmt` before committing Go code. `daemon/.golangci.yml` configures
`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, and `depguard`.
`depguard` checks the dependency direction described in
[architecture](../architecture.md#architecture-rules).

Run `make ci-backend` for Go changes. Use `make cover` when changing tested
backend behavior or its coverage map.

## Go 1.26

`daemon/go.mod` targets Go 1.26.0. For an optional value field that needs a
pointer, `new(value)` can replace a one-use pointer helper. This is a choice
for new code, not a reason to rewrite unrelated code. See the
[Go 1.26 release notes](https://go.dev/doc/go1.26).
