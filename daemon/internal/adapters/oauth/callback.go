// OAuth callback server: receiving one provider redirect.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	callbackHost              = "127.0.0.1"
	callbackReadHeaderTimeout = 5 * time.Second
	// callbackShutdownTimeout is how long a callback listener waits for the
	// page it is writing to reach the browser before it is dropped.
	callbackShutdownTimeout = 2 * time.Second
	callbackCodeQuery       = "code"
	callbackStateQuery      = "state"
	callbackErrorQuery      = "error"
	// callbackTicketQuery names the login a callback page is watching. It
	// replaces the grant in the address bar, so only the ticket stays there.
	callbackTicketQuery = "ticket"
)

const callbackStyle = `<style>
:root {
  color-scheme: light dark;
  --canvas: #f5f5f7; --surface: #fff;
  --ink: #17171c; --muted: #6b6b78;
  --line: rgb(0 0 0/.09);
  --shadow-1: 0 1px 2px rgb(20 20 30/.04);
  --ok: #19774e; --danger: #bd342a;
  --ok-bg: color-mix(in srgb, var(--ok) 12%, var(--surface));
  --danger-bg: color-mix(in srgb, var(--danger) 12%, var(--surface));
  --accent-base: #c8102e;
  --accent: var(--accent-base);
}
/* The page renders the default accent and follows the operating system theme:
   the redirect carries no session to read the operator's choice with. */
@media (prefers-color-scheme: dark) {
  :root {
    --canvas: #0a0a0a; --surface: #111113;
    --ink: #f2f2f5; --muted: #a3a3b0;
    --line: rgb(255 255 255/.08);
    --ok: #4cc38a; --danger: #ff7468;
    --accent: color-mix(in oklab, var(--accent-base) 72%, #fff);
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; min-height: 100vh; display: grid; place-items: center; padding: 28px;
  background: var(--canvas); color: var(--ink);
  font: 400 .9375rem/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
}
.panel {
  width: min(420px, 100%); padding: 18px;
  background: var(--surface); border: 1px solid var(--line); border-radius: 16px;
  box-shadow: var(--shadow-1);
  animation: page-in 180ms ease;
}
.glyph { color: var(--accent); display: block; }
h1 { margin: 16px 0 8px; font-size: 1rem; font-weight: 600; color: var(--ink); }
p { margin: 0; color: var(--muted); }
.mark {
  display: inline-flex; align-items: center; gap: 8px; margin-top: 16px;
  font-size: 12px; font-weight: 500;
  padding: 4px 10px; border-radius: 999px; background: var(--ok-bg); color: var(--ok);
}
.mark.failed { background: var(--danger-bg); color: var(--danger); }
.mark::before { content: ""; width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.foot { margin: 18px 0 0; font-size: 12px; color: var(--muted); }
@keyframes page-in { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: none; } }
@keyframes page-fade { from { opacity: 0; } to { opacity: 1; } }
body[data-closing="true"] .panel {
  opacity: 0; transform: translateY(6px);
  transition: opacity 200ms ease, transform 200ms ease;
}
@media (prefers-reduced-motion: reduce) {
  .panel { animation: page-fade 140ms ease; }
  body[data-closing="true"] .panel { transform: none; transition: opacity 140ms ease; }
}
</style>`

const callbackGlyph = `<svg class="glyph" width="32" height="32" viewBox="0 0 512 512" fill="none" aria-hidden="true">` +
	`<g stroke="currentColor" stroke-width="32" stroke-linecap="round" fill="none">` +
	`<path d="M256 91 A165 165 0 0 1 421 256"/>` +
	`<path d="M256 421 A165 165 0 0 1 91 256"/></g>` +
	`<circle cx="256" cy="256" r="64" fill="currentColor"/></svg>`

// CallbackIcon is the icon the callback pages carry in their own documents:
// the plated logo from assets/logo.svg, served as a data URI rather than
// fetched. A page that declares none leaves the browser to ask for a console
// icon, which may be another build's or a cached one. It is exported so the
// server's page carries the same artwork: the two callback templates must not
// diverge again the way a copied constant does.
const CallbackIcon = "data:image/svg+xml;base64," +
	"PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI1MTIiIGhlaWdodD0iNTEyIiB2aWV3Qm94PSIwIDAgNTEyIDUxMiIgcm9sZT0iaW1nIiBhcmlhLWxhYmVsPSJSZWxvIj4KICA8IS0tIFRoZSBSZWxvIGxvZ286IHRoZSBzcXVpcmNsZSBwbGF0ZSBjYXJyeWluZyB0aGUgbWFyay4gVGhpcyBmaWxlIGFuZAogICAgICAgYXNzZXRzL2xvZ28tbWFyay5zdmcgYXJlIHRoZSBvbmx5IGFydHdvcmsgYSBkZXNpZ25lciBlZGl0czsgbWFrZSBpY29ucwogICAgICAgZGVyaXZlcyBldmVyeSBpY29uLCBmYXZpY29uIGFuZCBpbnN0YWxsZXIgdGlsZSBmcm9tIHRoZW0uCgogICAgICAgVGhlIHBsYXRlIGlzIGEgc3VwZXJlbGxpcHNlIG9mIGV4cG9uZW50IDUuNSBmaWxsaW5nIHRoZSBjYW52YXMsIHNhbXBsZWQKICAgICAgIHJhdGhlciB0aGFuIGRyYXduIGFzIGEgcm91bmRlZCByZWN0IHNvIGl0cyBzaWxob3VldHRlIG1hdGNoZXMgdGhlIG1hY09TCiAgICAgICBpY29uIGdyaWQuIEl0cyBjb29yZGluYXRlcyBydW4gMCB0byA1MTIgcmF0aGVyIHRoYW4gc3RyYWRkbGluZyB0aGUKICAgICAgIG9yaWdpbjogdGhlIFNWRyByYXN0ZXJpc2VyIHN0b3BzIGEgcGF0aCBhdCBpdHMgZmlyc3QgbmVnYXRpdmUgbnVtYmVyLCBzbwogICAgICAgYSBwbGF0ZSBjZW50cmVkIG9uIHplcm8gcmVuZGVycyBhcyBvbmUgcXVhcnRlciBvZiBpdHNlbGYuCgogICAgICAgVGhlIG1hcmsgc2l0cyBhdCA2NiUgb2YgdGhlIHBsYXRlIHdpZHRoIGFuZCBpcyB0aGUgc2FtZSBnZW9tZXRyeSBhcwogICAgICAgbG9nby1tYXJrLnN2ZyB3aXRoIHRoZSBzY2FsZSBpbnRvIHRoZSBwYWdlIHdyaXR0ZW4gb3V0LCBiZWNhdXNlIHRoZQogICAgICAgU1ZHIHJhc3RlcmlzZXIgZHJvcHMgZWxlbWVudHMgY2FycnlpbmcgYSB0cmFuc2Zvcm0gYXR0cmlidXRlLiBUaGUgdHJheQogICAgICAgcmVhZHMgbG9nby1tYXJrLnN2ZyBvbiBpdHMgb3duLCBzbyB0aGUgdHdvIG11c3QgbmV2ZXIgZGlzYWdyZWUgYWJvdXQKICAgICAgIHRoZSBnZW9tZXRyeS4KCiAgICAgICBUaGUgc2hlZW4gYW5kIGRlcHRoIHdhc2hlcyBhcmUgdGhlIExpcXVpZCBHbGFzcyBmaW5pc2guIFRoZXkgYXJlCiAgICAgICBvcmRpbmFyeSB2ZXJ0aWNhbCBncmFkaWVudHMgd2l0aCBubyBnZW9tZXRyeSBvZiB0aGVpciBvd24sIHNvIGVkaXRpbmcKICAgICAgIHRoZSBicmFuZCBjb2xvdXIgYWJvdmUgdXBkYXRlcyB0aGUgd2hvbGUgcGxhdGUuIC0tPgogIDx0aXRsZT5SZWxvPC90aXRsZT4KICA8ZGVmcz4KICAgIDxsaW5lYXJHcmFkaWVudCBpZD0icGxhdGUiIHgxPSIwIiB5MT0iMCIgeDI9IjAiIHkyPSIxIj4KICAgICAgPHN0b3Agb2Zmc2V0PSIwIiBzdG9wLWNvbG9yPSIjZTAxODMzIi8+CiAgICAgIDxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iI2IwMGQyMiIvPgogICAgPC9saW5lYXJHcmFkaWVudD4KICAgIDxsaW5lYXJHcmFkaWVudCBpZD0ic2hlZW4iIHgxPSIwIiB5MT0iMCIgeDI9IjAiIHkyPSIxIj4KICAgIDxzdG9wIG9mZnNldD0iMC4wMCIgc3RvcC1vcGFjaXR5PSIwLjEzOSIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjA0IiBzdG9wLW9wYWNpdHk9IjAuMTU4Ii8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuMDgiIHN0b3Atb3BhY2l0eT0iMC4xNTgiLz4KICAgIDxzdG9wIG9mZnNldD0iMC4xMiIgc3RvcC1vcGFjaXR5PSIwLjEzOSIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjE2IiBzdG9wLW9wYWNpdHk9IjAuMTA4Ii8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuMjAiIHN0b3Atb3BhY2l0eT0iMC4wNzQiLz4KICAgIDxzdG9wIG9mZnNldD0iMC4yNCIgc3RvcC1vcGFjaXR5PSIwLjA0NSIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjI4IiBzdG9wLW9wYWNpdHk9IjAuMDI0Ii8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuMzIiIHN0b3Atb3BhY2l0eT0iMC4wMTEiLz4KICAgIDxzdG9wIG9mZnNldD0iMC4zNiIgc3RvcC1vcGFjaXR5PSIwLjAwNSIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjQwIiBzdG9wLW9wYWNpdHk9IjAuMDAyIi8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuNDQiIHN0b3Atb3BhY2l0eT0iMC4wMDAiLz4KICAgIDxzdG9wIG9mZnNldD0iMS4wMCIgc3RvcC1vcGFjaXR5PSIwLjAwMCIvPgogICAgPC9saW5lYXJHcmFkaWVudD4KICAgIDxsaW5lYXJHcmFkaWVudCBpZD0iZGVwdGgiIHgxPSIwIiB5MT0iMCIgeDI9IjAiIHkyPSIxIj4KICAgIDxzdG9wIG9mZnNldD0iMC4wMCIgc3RvcC1vcGFjaXR5PSIwLjAwMCIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjYwIiBzdG9wLW9wYWNpdHk9IjAuMDAwIi8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuNjQiIHN0b3Atb3BhY2l0eT0iMC4wMDAiLz4KICAgIDxzdG9wIG9mZnNldD0iMC42OCIgc3RvcC1vcGFjaXR5PSIwLjAwMiIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjcyIiBzdG9wLW9wYWNpdHk9IjAuMDA3Ii8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuNzYiIHN0b3Atb3BhY2l0eT0iMC4wMTQiLz4KICAgIDxzdG9wIG9mZnNldD0iMC44MCIgc3RvcC1vcGFjaXR5PSIwLjAyMiIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjg0IiBzdG9wLW9wYWNpdHk9IjAuMDM0Ii8+CiAgICA8c3RvcCBvZmZzZXQ9IjAuODgiIHN0b3Atb3BhY2l0eT0iMC4wNDciLz4KICAgIDxzdG9wIG9mZnNldD0iMC45MiIgc3RvcC1vcGFjaXR5PSIwLjA2MiIvPgogICAgPHN0b3Agb2Zmc2V0PSIwLjk2IiBzdG9wLW9wYWNpdHk9IjAuMDgwIi8+CiAgICA8c3RvcCBvZmZzZXQ9IjEuMDAiIHN0b3Atb3BhY2l0eT0iMC4xMDAiLz4KICAgIDwvbGluZWFyR3JhZGllbnQ+CiAgICA8Y2xpcFBhdGggaWQ9InNxdWlyY2xlIj4KICAgICAgPHBhdGggZD0iTTUxMi4wMCAyNTYuMDBMNTExLjg5IDM0MS41NEw1MTEuNTUgMzY2LjAxTDUxMC45OSAzODMuNDBMNTEwLjIwIDM5Ny4zMEw1MDkuMTggNDA5LjA0TDUwNy45NCA0MTkuMjdMNTA2LjQ1IDQyOC4zNUw1MDQuNzMgNDM2LjUzTDUwMi43NyA0NDMuOTZMNTAwLjU3IDQ1MC43NUw0OTguMTAgNDU2Ljk5TDQ5NS4zOCA0NjIuNzRMNDkyLjM5IDQ2OC4wNUw0ODkuMTIgNDcyLjk2TDQ4NS41NiA0NzcuNDlMNDgxLjY5IDQ4MS42OUw0NzcuNDkgNDg1LjU2TDQ3Mi45NiA0ODkuMTJMNDY4LjA1IDQ5Mi4zOUw0NjIuNzQgNDk1LjM4TDQ1Ni45OSA0OTguMTBMNDUwLjc1IDUwMC41N0w0NDMuOTYgNTAyLjc3TDQzNi41MyA1MDQuNzNMNDI4LjM1IDUwNi40NUw0MTkuMjcgNTA3Ljk0TDQwOS4wNCA1MDkuMThMMzk3LjMwIDUxMC4yMEwzODMuNDAgNTEwLjk5TDM2Ni4wMSA1MTEuNTVMMzQxLjU0IDUxMS44OUwyNTYuMDAgNTEyLjAwTDE3MC40NiA1MTEuODlMMTQ1Ljk5IDUxMS41NUwxMjguNjAgNTEwLjk5TDExNC43MCA1MTAuMjBMMTAyLjk2IDUwOS4xOEw5Mi43MyA1MDcuOTRMODMuNjUgNTA2LjQ1TDc1LjQ3IDUwNC43M0w2OC4wNCA1MDIuNzdMNjEuMjUgNTAwLjU3TDU1LjAxIDQ5OC4xMEw0OS4yNiA0OTUuMzhMNDMuOTUgNDkyLjM5TDM5LjA0IDQ4OS4xMkwzNC41MSA0ODUuNTZMMzAuMzEgNDgxLjY5TDI2LjQ0IDQ3Ny40OUwyMi44OCA0NzIuOTZMMTkuNjEgNDY4LjA1TDE2LjYyIDQ2Mi43NEwxMy45MCA0NTYuOTlMMTEuNDMgNDUwLjc1TDkuMjMgNDQzLjk2TDcuMjcgNDM2LjUzTDUuNTUgNDI4LjM1TDQuMDYgNDE5LjI3TDIuODIgNDA5LjA0TDEuODAgMzk3LjMwTDEuMDEgMzgzLjQwTDAuNDUgMzY2LjAxTDAuMTEgMzQxLjU0TDAuMDAgMjU2LjAwTDAuMTEgMTcwLjQ2TDAuNDUgMTQ1Ljk5TDEuMDEgMTI4LjYwTDEuODAgMTE0LjcwTDIuODIgMTAyLjk2TDQuMDYgOTIuNzNMNS41NSA4My42NUw3LjI3IDc1LjQ3TDkuMjMgNjguMDRMMTEuNDMgNjEuMjVMMTMuOTAgNTUuMDFMMTYuNjIgNDkuMjZMMTkuNjEgNDMuOTVMMjIuODggMzkuMDRMMjYuNDQgMzQuNTFMMzAuMzEgMzAuMzFMMzQuNTEgMjYuNDRMMzkuMDQgMjIuODhMNDMuOTUgMTkuNjFMNDkuMjYgMTYuNjJMNTUuMDEgMTMuOTBMNjEuMjUgMTEuNDNMNjguMDQgOS4yM0w3NS40NyA3LjI3TDgzLjY1IDUuNTVMOTIuNzMgNC4wNkwxMDIuOTYgMi44MkwxMTQuNzAgMS44MEwxMjguNjAgMS4wMUwxNDUuOTkgMC40NUwxNzAuNDYgMC4xMUwyNTYuMDAgMC4wMEwzNDEuNTQgMC4xMUwzNjYuMDEgMC40NUwzODMuNDAgMS4wMUwzOTcuMzAgMS44MEw0MDkuMDQgMi44Mkw0MTkuMjcgNC4wNkw0MjguMzUgNS41NUw0MzYuNTMgNy4yN0w0NDMuOTYgOS4yM0w0NTAuNzUgMTEuNDNMNDU2Ljk5IDEzLjkwTDQ2Mi43NCAxNi42Mkw0NjguMDUgMTkuNjFMNDcyLjk2IDIyLjg4TDQ3Ny40OSAyNi40NEw0ODEuNjkgMzAuMzFMNDg1LjU2IDM0LjUxTDQ4OS4xMiAzOS4wNEw0OTIuMzkgNDMuOTVMNDk1LjM4IDQ5LjI2TDQ5OC4xMCA1NS4wMUw1MDAuNTcgNjEuMjVMNTAyLjc3IDY4LjA0TDUwNC43MyA3NS40N0w1MDYuNDUgODMuNjVMNTA3Ljk0IDkyLjczTDUwOS4xOCAxMDIuOTZMNTEwLjIwIDExNC43MEw1MTAuOTkgMTI4LjYwTDUxMS41NSAxNDUuOTlMNTExLjg5IDE3MC40NloiLz4KICAgIDwvY2xpcFBhdGg+CiAgPC9kZWZzPgogIDxwYXRoIGQ9Ik01MTIuMDAgMjU2LjAwTDUxMS44OSAzNDEuNTRMNTExLjU1IDM2Ni4wMUw1MTAuOTkgMzgzLjQwTDUxMC4yMCAzOTcuMzBMNTA5LjE4IDQwOS4wNEw1MDcuOTQgNDE5LjI3TDUwNi40NSA0MjguMzVMNTA0LjczIDQzNi41M0w1MDIuNzcgNDQzLjk2TDUwMC41NyA0NTAuNzVMNDk4LjEwIDQ1Ni45OUw0OTUuMzggNDYyLjc0TDQ5Mi4zOSA0NjguMDVMNDg5LjEyIDQ3Mi45Nkw0ODUuNTYgNDc3LjQ5TDQ4MS42OSA0ODEuNjlMNDc3LjQ5IDQ4NS41Nkw0NzIuOTYgNDg5LjEyTDQ2OC4wNSA0OTIuMzlMNDYyLjc0IDQ5NS4zOEw0NTYuOTkgNDk4LjEwTDQ1MC43NSA1MDAuNTdMNDQzLjk2IDUwMi43N0w0MzYuNTMgNTA0LjczTDQyOC4zNSA1MDYuNDVMNDE5LjI3IDUwNy45NEw0MDkuMDQgNTA5LjE4TDM5Ny4zMCA1MTAuMjBMMzgzLjQwIDUxMC45OUwzNjYuMDEgNTExLjU1TDM0MS41NCA1MTEuODlMMjU2LjAwIDUxMi4wMEwxNzAuNDYgNTExLjg5TDE0NS45OSA1MTEuNTVMMTI4LjYwIDUxMC45OUwxMTQuNzAgNTEwLjIwTDEwMi45NiA1MDkuMThMOTIuNzMgNTA3Ljk0TDgzLjY1IDUwNi40NUw3NS40NyA1MDQuNzNMNjguMDQgNTAyLjc3TDYxLjI1IDUwMC41N0w1NS4wMSA0OTguMTBMNDkuMjYgNDk1LjM4TDQzLjk1IDQ5Mi4zOUwzOS4wNCA0ODkuMTJMMzQuNTEgNDg1LjU2TDMwLjMxIDQ4MS42OUwyNi40NCA0NzcuNDlMMjIuODggNDcyLjk2TDE5LjYxIDQ2OC4wNUwxNi42MiA0NjIuNzRMMTMuOTAgNDU2Ljk5TDExLjQzIDQ1MC43NUw5LjIzIDQ0My45Nkw3LjI3IDQzNi41M0w1LjU1IDQyOC4zNUw0LjA2IDQxOS4yN0wyLjgyIDQwOS4wNEwxLjgwIDM5Ny4zMEwxLjAxIDM4My40MEwwLjQ1IDM2Ni4wMUwwLjExIDM0MS41NEwwLjAwIDI1Ni4wMEwwLjExIDE3MC40NkwwLjQ1IDE0NS45OUwxLjAxIDEyOC42MEwxLjgwIDExNC43MEwyLjgyIDEwMi45Nkw0LjA2IDkyLjczTDUuNTUgODMuNjVMNy4yNyA3NS40N0w5LjIzIDY4LjA0TDExLjQzIDYxLjI1TDEzLjkwIDU1LjAxTDE2LjYyIDQ5LjI2TDE5LjYxIDQzLjk1TDIyLjg4IDM5LjA0TDI2LjQ0IDM0LjUxTDMwLjMxIDMwLjMxTDM0LjUxIDI2LjQ0TDM5LjA0IDIyLjg4TDQzLjk1IDE5LjYxTDQ5LjI2IDE2LjYyTDU1LjAxIDEzLjkwTDYxLjI1IDExLjQzTDY4LjA0IDkuMjNMNzUuNDcgNy4yN0w4My42NSA1LjU1TDkyLjczIDQuMDZMMTAyLjk2IDIuODJMMTE0LjcwIDEuODBMMTI4LjYwIDEuMDFMMTQ1Ljk5IDAuNDVMMTcwLjQ2IDAuMTFMMjU2LjAwIDAuMDBMMzQxLjU0IDAuMTFMMzY2LjAxIDAuNDVMMzgzLjQwIDEuMDFMMzk3LjMwIDEuODBMNDA5LjA0IDIuODJMNDE5LjI3IDQuMDZMNDI4LjM1IDUuNTVMNDM2LjUzIDcuMjdMNDQzLjk2IDkuMjNMNDUwLjc1IDExLjQzTDQ1Ni45OSAxMy45MEw0NjIuNzQgMTYuNjJMNDY4LjA1IDE5LjYxTDQ3Mi45NiAyMi44OEw0NzcuNDkgMjYuNDRMNDgxLjY5IDMwLjMxTDQ4NS41NiAzNC41MUw0ODkuMTIgMzkuMDRMNDkyLjM5IDQzLjk1TDQ5NS4zOCA0OS4yNkw0OTguMTAgNTUuMDFMNTAwLjU3IDYxLjI1TDUwMi43NyA2OC4wNEw1MDQuNzMgNzUuNDdMNTA2LjQ1IDgzLjY1TDUwNy45NCA5Mi43M0w1MDkuMTggMTAyLjk2TDUxMC4yMCAxMTQuNzBMNTEwLjk5IDEyOC42MEw1MTEuNTUgMTQ1Ljk5TDUxMS44OSAxNzAuNDZaIiBmaWxsPSJ1cmwoI3BsYXRlKSIvPgogIDxnIGNsaXAtcGF0aD0idXJsKCNzcXVpcmNsZSkiPgogICAgPHJlY3Qgd2lkdGg9IjUxMiIgaGVpZ2h0PSI1MTIiIGZpbGw9InVybCgjc2hlZW4pIi8+CiAgICA8cmVjdCB3aWR0aD0iNTEyIiBoZWlnaHQ9IjUxMiIgZmlsbD0idXJsKCNkZXB0aCkiLz4KICA8L2c+CiAgPCEtLSBUaGUgYnJhbmQgbWFyaywgc2NhbGVkIHRvIDY2JSBvZiB0aGUgcGxhdGUgYW5kIGNlbnRyZWQuIFRoZSBudW1iZXJzIGFyZQogICAgICAgbG9nby1tYXJrLnN2ZyB3aXRoIHRoZSBzY2FsZSBhbmQgb2Zmc2V0IGFwcGxpZWQgYnkgaGFuZCwgYmVjYXVzZSB0aGUgU1ZHCiAgICAgICByYXN0ZXJpc2VyIGRyb3BzIGV2ZXJ5IGVsZW1lbnQgdGhhdCBjYXJyaWVzIGEgdHJhbnNmb3JtIGF0dHJpYnV0ZS4gLS0+CiAgPGcgc3Ryb2tlPSIjZmZmZmZmIiBzdHJva2Utd2lkdGg9IjIxLjEyIiBzdHJva2UtbGluZWNhcD0icm91bmQiIGZpbGw9Im5vbmUiPgogICAgPHBhdGggZD0iTTI1Ni4wMCAxNDcuMTAgQTEwOC45MCAxMDguOTAgMCAwIDEgMzY0LjkwIDI1Ni4wMCIvPgogICAgPHBhdGggZD0iTTI1Ni4wMCAzNjQuOTAgQTEwOC45MCAxMDguOTAgMCAwIDEgMTQ3LjEwIDI1Ni4wMCIvPgogIDwvZz4KICA8Y2lyY2xlIGN4PSIyNTYiIGN5PSIyNTYiIHI9IjQyLjI0IiBmaWxsPSIjZmZmZmZmIi8+Cjwvc3ZnPgo="

const callbackPageScript = `<script>try{history.replaceState(null,"",location.pathname)}catch(e){}setTimeout(function(){document.body.setAttribute("data-closing","true")},2800);setTimeout(function(){window.close()},3000)</script>`

var (
	callbackPage = callbackDocument("Authorization received",
		"Relo received the authorization. It is exchanging it now, which a moment longer takes; the Access page reports the result and the account it stored.",
		"Connected", false)
	callbackErrorPage = callbackDocument("Relo could not finish the login",
		"The provider refused the authorization or it did not match the login Relo started. Start the connection again from the Access page.",
		"Not connected", true)
)

func callbackDocument(title, summary, marker string, failed bool) string {
	class := "mark"
	if failed {
		class = "mark failed"
	}
	return "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">" +
		"<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">" +
		"<title>" + title + " · Relo</title>" +
		"<link rel=\"icon\" href=\"" + CallbackIcon + "\">" + callbackStyle + "</head><body>" +
		"<main class=\"panel\">" + callbackGlyph + "<h1>" + title + "</h1><p>" + summary + "</p>" +
		"<span class=\"" + class + "\">" + marker + "</span>" +
		"<p class=\"foot\">You can close this tab and return to Relo.</p>" +
		"</main>" + callbackPageScript + "</body></html>"
}

// CallbackResult carries the authorization code and state one redirect
// delivered.
type CallbackResult struct {
	Code  string
	State string
}

// CallbackServer is the loopback listener waiting for one OAuth
// redirect. The listener is closed on every path out of Wait.
type CallbackServer struct {
	listener net.Listener
	server   *http.Server
	path     string
	state    string
	outcomes chan callbackOutcome
	closed   sync.Once
	stopped  atomic.Bool
	// broker and provider turn this listener into a bridge: the provider
	// redirects here, this listener hands the answer to the login waiting on
	// the daemon, and the browser is sent on to the page that reports it.
	broker   *CallbackBroker
	provider string
}

type callbackOutcome struct {
	result CallbackResult
	err    error
}

// NewCallbackServer binds a loopback callback port for one login. Port
// zero lets the operating system pick a free one.
func NewCallbackServer(port int, path, state string) (*CallbackServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", callbackHost, port))
	if err != nil {
		return nil, fmt.Errorf("callback port %d: %w", port, ErrPortBusy)
	}
	server := &CallbackServer{
		listener: listener,
		path:     path,
		state:    state,
		outcomes: make(chan callbackOutcome, 1),
	}
	server.server = &http.Server{Handler: server, ReadHeaderTimeout: callbackReadHeaderTimeout}
	go func() { _ = server.server.Serve(listener) }()
	return server, nil
}

// Port returns the port the listener bound.
func (s *CallbackServer) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// RedirectURI returns the redirect_uri the provider must call back on.
func (s *CallbackServer) RedirectURI(host string) string {
	return fmt.Sprintf("http://%s:%d%s", option(host, callbackHost), s.Port(), s.path)
}

func (s *CallbackServer) forwardTo(broker *CallbackBroker, provider string) {
	s.broker, s.provider = broker, provider
}

// Wait returns the first callback the browser delivers and closes the
// listener before returning either the result or an error.
func (s *CallbackServer) Wait(ctx context.Context) (*CallbackResult, error) {
	defer func() { _ = s.Close() }()
	select {
	case outcome := <-s.outcomes:
		if outcome.err != nil {
			return nil, outcome.err
		}
		return &outcome.result, nil
	case <-ctx.Done():
		return nil, callbackWaitError(ctx, s.Port())
	}
}

// Close stops the listener. Closing more than once is safe. The listener is
// shut down rather than dropped, because the login finishes as soon as the
// outcome is handed over: a page still being written when that happens has to
// reach the browser, and a dropped connection shows it as nothing at all.
func (s *CallbackServer) Close() error {
	var err error
	s.closed.Do(func() {
		s.stopped.Store(true)
		ctx, cancel := context.WithTimeout(context.Background(), callbackShutdownTimeout)
		defer cancel()
		if err = s.server.Shutdown(ctx); err != nil {
			err = s.server.Close()
		}
	})
	return err
}

// ServeHTTP answers the provider redirect and serves the page that
// tells the human to close the tab.
func (s *CallbackServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.stopped.Load() || r.URL.Path != s.path {
		http.NotFound(w, r)
		return
	}
	if s.broker != nil {
		s.forward(w, r)
		return
	}
	result, err := parseCallbackQuery(r.URL.Query(), s.state)
	s.deliver(callbackOutcome{result: result, err: err})
	setCallbackHeaders(w)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, callbackErrorPage)
		return
	}
	_, _ = io.WriteString(w, callbackPage)
}

func (s *CallbackServer) forward(w http.ResponseWriter, r *http.Request) {
	setCallbackHeaders(w)
	ticket, err := s.broker.Deliver(s.provider, r.URL.Query())
	target := s.broker.PageURL(s.provider)
	if ticket != "" {
		target = s.broker.TicketURL(s.provider, ticket)
	}
	// err is the reason no code was handed over. The page the browser is
	// sent to reports it from the login itself, so nothing here has to.
	_ = err
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func setCallbackHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

func (s *CallbackServer) deliver(outcome callbackOutcome) {
	select {
	case s.outcomes <- outcome:
	default:
	}
}

// ListenForCallback waits up to timeout for the browser to deliver one
// authorization callback on the given loopback port and path.
func ListenForCallback(port int, path string, timeout time.Duration) (*CallbackResult, error) {
	server, err := NewCallbackServer(port, path, "")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return server.Wait(ctx)
}

func callbackWaitError(ctx context.Context, port int) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("callback on port %d: %w", port, ErrCallbackTimeout)
	}
	return fmt.Errorf("callback on port %d: %w", port, ErrLoginCancelled)
}

func parseCallbackQuery(query url.Values, expectedState string) (CallbackResult, error) {
	if failure := query.Get(callbackErrorQuery); failure != "" {
		return CallbackResult{}, fmt.Errorf("the provider refused the login (%s): %w", failure, ErrLoginCancelled)
	}
	code := strings.TrimSpace(query.Get(callbackCodeQuery))
	if code == "" {
		return CallbackResult{}, ErrMissingCode
	}
	state := strings.TrimSpace(query.Get(callbackStateQuery))
	if expectedState != "" && state != expectedState {
		return CallbackResult{}, ErrInvalidState
	}
	return CallbackResult{Code: code, State: state}, nil
}

func parseCallbackAnswer(answer, expectedState string) (CallbackResult, error) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return CallbackResult{}, ErrMissingCode
	}
	if !strings.Contains(trimmed, "=") {
		return CallbackResult{Code: trimmed}, nil
	}
	values, err := answerQuery(trimmed)
	if err != nil {
		return CallbackResult{}, err
	}
	return parseCallbackQuery(values, expectedState)
}

func answerQuery(answer string) (url.Values, error) {
	if parsed, err := url.Parse(answer); err == nil && parsed.Scheme != "" {
		return parsed.Query(), nil
	}
	values, err := url.ParseQuery(answer)
	if err != nil {
		return nil, fmt.Errorf("parse the authorization answer: %w", err)
	}
	return values, nil
}
