# Principles

Shared engineering principles for Relo. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the contribution workflow.

## Clean Code

Code should explain itself through names, structure, and small functions. Use
comments for a non-obvious reason or constraint, not to narrate a line. For
example, name a function `resolveAccount` instead of commenting `// find the
account` above an unexplained block. Remove only dead code created by your
change; leave unrelated cleanup for its own change.

## Clean Architecture

Business rules belong to the feature that owns them; external systems belong
to adapters. Dependencies flow inward and wiring happens at the composition
root. For example, routing asks a small account interface for candidates;
routing does not query SQLite or talk to a provider itself.

## KISS

Write the least code that solves the observed problem. Introduce a new
abstraction when real callers need it. For example, keep a single provider
operation as a function until a second implementation makes an interface
useful.

## DRY

Extract repeated *behavior* into a named helper in the package that owns the
concept. Do not merge blocks that only look alike but answer different
business questions; a shared function with mode flags is often harder to
read than the duplication.

## SOLID

Keep a component or package focused on one responsibility. Extend through
small, substitutable interfaces when there are real variants; let consumers
declare the operations they need and inject implementations at the boundary.
For example, a read-only account view should depend on an account reader,
not a whole store with write methods. Apply these principles to clarify
relationships, not to create abstraction layers in advance.

## Quick Reference

| Principle          | Rule                                                               |
|--------------------|--------------------------------------------------------------------|
| Clean Code         | Prefer clear names and structure; comment on why when needed.      |
| Clean Architecture | Keep feature rules inward and adapters at the edge.                |
| KISS               | Solve the present problem with the least necessary code.           |
| DRY                | Share real behavior, not merely similar-looking code.              |
| SOLID              | Keep responsibilities focused and interfaces small and useful.     |
| Tests              | Check observable behavior and relevant failure paths.              |
| Commits            | Conventional prefix, one logical change, under 72 characters.      |
| CI                 | Run checks relevant to the change; `make ci` covers the full gate. |
