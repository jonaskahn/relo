# Security Policy

## Supported versions

Security fixes go into the latest release. Upgrade before reporting an issue
that may already be fixed.

## Reporting a vulnerability

**Do not open a public issue, discussion, or pull request for a vulnerability.**

Report it privately through GitHub:
<https://github.com/jonaskahn/relo/security/advisories/new>

Include the affected version, your platform, the impact, and steps to
reproduce. A proof of concept helps. Do not include real credentials.

You can expect an acknowledgement within 7 days. We will keep you updated while
we investigate and fix the issue, and credit you in the release notes unless
you prefer otherwise. Please give us a reasonable chance to ship a fix before
disclosing publicly.

## Security model

What Relo protects, so you can judge whether something is a vulnerability:

- **Loopback by default.** The daemon binds to `127.0.0.1`. Turning the admin
  sign-in off on a non-loopback bind is refused at startup unless
  `admin.allow_external` is set.
- **Separate credentials.** Client keys reach only the data plane. The admin
  token reaches only the management API and console. Upstream credentials are
  never sent to agents.
- **Secrets at rest.** Account credentials live in the OS keychain, or in
  `$RELO_HOME/secrets.enc` sealed by `$RELO_HOME/secret.key`.
- **CSRF.** Every management API write needs an `X-CSRF-Token` header matching
  the `relo_csrf` cookie.
- **Captured traffic is redacted** before it is stored.

## In scope

- Authentication or authorization bypass on the management API or data plane
- Credential or secret disclosure (logs, captures, API responses, files)
- CSRF, request forgery, or DNS-rebinding access to a loopback listener
- Remote code execution or arbitrary file access through the daemon, installer,
  or updater

## Out of scope

- Attacks that need an attacker already running as the same OS user, or with
  read access to `$RELO_HOME`
- Deliberate misconfiguration, such as exposing the daemon with the admin
  sign-in off and `admin.allow_external` on
- Vulnerabilities in upstream providers or coding clients themselves
- Denial of service by a local user of a loopback-only daemon

Vulnerabilities in a third-party dependency should also be reported to that
project; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) for what we ship.
