# Contributing to Relo

Thanks for helping make Relo better. This page covers the basics; the detailed
rules live in [`docs/`](docs/README.md).

## Ground rules

- Write code, comments, commits, and docs in English.
- Report security problems privately, following [SECURITY.md](SECURITY.md).
  Never open a public issue for a vulnerability.
- Keep changes small and focused: one logical change per commit and per pull
  request.
- Check an existing issue or open one before starting a large change.

## Getting started

Requirements: Go 1.26+ and Node 20+. No CGO.

```sh
git clone https://github.com/jonaskahn/relo.git
cd relo
make ci
```

Windows contributors: see [Development](docs/development.md#windows-contributors).

## Before you write code

Read what applies to your change:

| Working on                 | Read                                                                                                                 |
|----------------------------|----------------------------------------------------------------------------------------------------------------------|
| Anything                   | [Principles](docs/principles.md), [Glossary](docs/glossary.md)                                                       |
| Backend structure or rules | [Architecture](docs/architecture.md), [Go conventions](docs/conventions/golang.md)                                   |
| Management API             | [API conventions](docs/api.md)                                                                                       |
| User-facing text, artwork  | [Product rules](docs/conventions/product-rules.md)                                                                   |
| Console                    | [Svelte conventions](docs/conventions/svelte.md), [Product](docs/design/PRODUCT.md), [Design](docs/design/DESIGN.md) |

## Making a change

1. Fork the repository and create a branch from `main`.
2. Make your change, with tests for observable behavior and failure paths.
3. Run the checks that match your change (see [Development](docs/development.md)):
   `make ci-backend` for Go, `make ci-console` for the console, `make ci` for
   everything.
4. Commit, then open a pull request.

### Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/). Keep the
subject under 72 characters.

| Prefix      | Use                                             |
|-------------|-------------------------------------------------|
| `feat:`     | New user-visible functionality                  |
| `fix:`      | Bug fix                                         |
| `refactor:` | Code change that is neither a fix nor a feature |
| `test:`     | Adding or updating tests only                   |
| `docs:`     | Documentation only                              |
| `chore:`    | Build, CI, dependency updates                   |

### Pull requests

The description answers four questions: What changed? Why? How was it tested?
What are the risks or limitations?

## Licenses

Bundled third-party software is listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Update it when you add or
upgrade a dependency, font, or logo.
