// Callback page view: the wording and state a browser landing shows.
package server

import (
	"html/template"
	"net/http"

	"github.com/jonaskahn/relo/internal/i18n"
)

const callbackConsoleURL = "/providers"

const callbackCloseSeconds = 3

const callbackSecondsMarker = "__SECONDS__"

type callbackView struct {
	Language string
	Provider string
	Heading  string
	Summary  string
	Marker   string
	// StatusURL is where the script polls. It is empty for a page that
	// already renders a final state, which is what a redirect with no login
	// left to follow produces.
	StatusURL string
	// CloseSeconds is how long the page counts down before closing itself,
	// once the login has an outcome.
	CloseSeconds int
	// Failed and Expired are the two end states a redirect renders straight
	// away: a login that did not finish, and one this process no longer
	// tracks.
	Failed    bool
	Expired   bool
	Connected bool
	// Account is the label of the account a finished login stored.
	Account string
	// Wording is the prose the script swaps in as a login finishes.
	Wording callbackWording
}

type callbackWording struct {
	AccountLabel   string
	ConsoleLabel   string
	ConsoleURL     string
	CloseHint      string
	StepsLabel     string
	StepAuthorized string
	StepExchanging string
	StepStored     string
	ConnectedTitle string
	ConnectedBody  string
	FailedTitle    string
	ExpiredTitle   string
	ExpiredBody    string
	// ClosingIn is the sentence the countdown renders, with the seconds left
	// as a placeholder the page replaces each tick.
	ClosingIn       string
	MarkerConnected string
	MarkerFailed    string
	MarkerWorking   string
	FailureDenied   string
	FailureState    string
	FailureTimeout  string
	FailureExchange string
}

func (s *Server) callbackView(r *http.Request, provider, ticket string) callbackView {
	translator := s.translator(r)
	provider = callbackProviderName(provider)
	text := func(id string) string { return translator.Text(id, nil) }
	return callbackView{
		Language:     translator.Language(),
		Provider:     provider,
		Heading:      text("api.callback.heading"),
		Summary:      text("api.callback.summary"),
		Marker:       text("api.callback.marker_working"),
		StatusURL:    callbackStatusURL(provider, ticket),
		CloseSeconds: callbackCloseSeconds,
		Wording:      callbackWordingOf(translator, text, provider),
	}
}

func callbackWordingOf(translator *i18n.Translator, text func(string) string, provider string) callbackWording {
	return callbackWording{
		AccountLabel: text("api.callback.account_label"),
		ConsoleLabel: text("api.callback.console_link"),
		ConsoleURL:   callbackConsoleURL,
		CloseHint:    text("api.callback.close_hint"),
		StepsLabel:   text("api.callback.steps_label"),
		StepAuthorized: translator.Text("api.callback.step_authorized",
			map[string]any{"Provider": provider}),
		StepExchanging: text("api.callback.step_exchanging"),
		StepStored:     text("api.callback.step_stored"),
		ConnectedTitle: text("api.callback.connected_title"),
		ConnectedBody:  text("api.callback.connected_body"),
		FailedTitle:    text("api.callback.failed_title"),
		ExpiredTitle:   text("api.callback.expired_title"),
		ExpiredBody:    text("api.callback.expired_body"),
		ClosingIn: translator.Text("api.callback.closing_in",
			map[string]any{"Seconds": callbackSecondsMarker}),
		MarkerConnected: text("api.callback.marker_connected"),
		MarkerFailed:    text("api.callback.marker_failed"),
		MarkerWorking:   text("api.callback.marker_working"),
		FailureDenied:   text("api.callback.failure_denied"),
		FailureState:    text("api.callback.failure_state"),
		FailureTimeout:  text("api.callback.failure_timeout"),
		FailureExchange: text("api.callback.failure_exchange"),
	}
}

func callbackStatusURL(provider, ticket string) string {
	if ticket == "" {
		return ""
	}
	return CallbackPrefix + provider + "/status?" + callbackTicketQuery + "=" + ticket
}

func callbackProviderName(provider string) string {
	if provider == "" {
		return "Relo"
	}
	return provider
}

func callbackFailureBody(wording callbackWording, category CallbackOutcome) string {
	switch category {
	case CallbackDenied:
		return wording.FailureDenied
	case CallbackState:
		return wording.FailureState
	case CallbackTimeout:
		return wording.FailureTimeout
	default:
		return wording.FailureExchange
	}
}

// callbackIconPlaceholder marks where the brand icon goes. The page fills
// it in after rendering with the icon the platform injected.
const callbackIconPlaceholder = "__RELO_CALLBACK_ICON__"

// callbackTemplate and callbackDocument are the page a build without a console
// renders. A build that embedded one serves the console's callback route
// instead, which carries this page's wording in the console catalogs and its
// motion in Svelte; what is here is what a binary compiled with no dashboard
// still lands on.
var callbackTemplate = template.Must(template.New("callback").Parse(callbackDocument))

var callbackDocument = `<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{.Heading}} · Relo</title>
<link rel="icon" href="__RELO_CALLBACK_ICON__">
<style>
:root {
  color-scheme: light dark;
  --canvas: #f5f5f7; --surface: #fff; --raised: #fff; --sunken: #f5f5f7;
  --ink: #17171c; --muted: #6b6b78; --faint: #858592;
  --line: rgb(0 0 0/.09); --line-strong: rgb(0 0 0/.16);
  --shadow: rgb(20 20 40/.10); --shadow-1: 0 1px 2px rgb(20 20 30/.04);
  --ok: #19774e; --warn: #9b5800; --danger: #bd342a;
  --ok-bg: color-mix(in srgb, var(--ok) 12%, var(--surface));
  --danger-bg: color-mix(in srgb, var(--danger) 12%, var(--surface));
  --accent-base: #c8102e; --on-accent: #fff; --shade: #000; --mixbase: var(--surface);
  --t50: 7%; --t100: 14%; --t200: 26%;
  --accent: var(--accent-base);
  --accent-50: color-mix(in oklab, var(--accent) var(--t50), var(--mixbase));
  --accent-700: color-mix(in oklab, var(--accent) 72%, var(--shade));
  --accent-ink: color-mix(in oklab, var(--accent) 56%, var(--shade));
  --accent-border: color-mix(in oklab, var(--accent) 32%, var(--mixbase));
}
/* The page renders the default accent and follows the operating system theme:
   the redirect carries no session to read the operator's choice with. */
[data-accent='red'] { --accent-base: #c8102e; --on-accent: #fff; }
[data-accent='orange'] { --accent-base: #d9622b; --on-accent: #17171c; }
[data-accent='amber'] { --accent-base: #cc7c2e; --on-accent: #17171c; }
[data-accent='green'] { --accent-base: #43946c; --on-accent: #17171c; }
[data-accent='teal'] { --accent-base: #439288; --on-accent: #17171c; }
[data-accent='cyan'] { --accent-base: #418fae; --on-accent: #17171c; }
[data-accent='blue'] { --accent-base: #3662e3; --on-accent: #fff; }
[data-accent='indigo'] { --accent-base: #4e46dc; --on-accent: #fff; }
[data-accent='purple'] { --accent-base: #743ee4; --on-accent: #fff; }
[data-accent='rose'] { --accent-base: #cf364c; --on-accent: #fff; }
@media (prefers-color-scheme: dark) {
  :root {
    --canvas: #0a0a0a; --surface: #111113; --raised: #1a1a1d; --sunken: #0a0a0a;
    --ink: #f2f2f5; --muted: #a3a3b0; --faint: #6f6f7c;
    --line: rgb(255 255 255/.08); --line-strong: rgb(255 255 255/.16);
    --shadow: rgb(0 0 0/.55);
    --ok: #4cc38a; --warn: #f0a93b; --danger: #ff7468;
    --shade: #fff; --t50: 12%; --t100: 22%; --t200: 34%;
    --accent: color-mix(in oklab, var(--accent-base) 72%, #fff); --on-accent: #0a0a0a;
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; min-height: 100vh; display: grid; place-items: center; padding: 28px;
  background: var(--canvas); color: var(--ink);
  font: 400 .9375rem/1.5 system-ui, -apple-system, "Segoe UI", sans-serif;
}
.panel {
  width: min(460px, 100%); padding: 18px;
  background: var(--surface); border: 1px solid var(--line); border-radius: 16px;
  box-shadow: var(--shadow-1);
  animation: page-in 180ms ease;
}
.glyph { color: var(--accent); display: block; }
.provider {
  margin: 16px 0 0; font-size: 12px; font-weight: 500; color: var(--muted);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
h1 { margin: 4px 0 8px; font-size: 1rem; font-weight: 600; color: var(--ink); }
#summary { margin: 0; max-width: 72ch; color: var(--muted); }
.mark {
  display: inline-flex; align-items: center; gap: 8px; margin-top: 16px;
  font-size: 12px; font-weight: 500;
  padding: 4px 10px; border-radius: 999px; background: var(--accent-50); color: var(--accent-ink);
  transition: background-color 150ms ease, color 150ms ease;
}
.mark::before { content: ""; width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.mark[data-state="connected"] { background: var(--ok-bg); color: var(--ok); }
.mark[data-state="failed"] { background: var(--danger-bg); color: var(--danger); }
.steps { list-style: none; margin: 18px 0 0; padding: 0; display: grid; gap: 8px; }
.steps li {
  display: flex; align-items: center; gap: 10px;
  padding: 9px 12px; border: 1px solid var(--line); border-radius: 10px;
  background: var(--surface); color: var(--muted); font-size: .875rem;
}
.steps li::before {
  content: ""; flex: none; width: 8px; height: 8px; border-radius: 50%;
  background: var(--line-strong);
}
.steps li[data-state="done"] { color: var(--ink); }
.steps li[data-state="done"]::before { background: var(--ok); }
.steps li[data-state="active"] { color: var(--ink); border-color: var(--accent-border); }
.steps li[data-state="active"]::before { background: var(--accent); }
.steps li[data-state="failed"] { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 30%, var(--line)); }
.steps li[data-state="failed"]::before { background: var(--danger); }
.steps li[data-state="skipped"] { display: none; }
.steps li[data-state="done"],
.steps li[data-state="active"],
.steps li[data-state="failed"] { animation: step-in 180ms ease; }
.account { margin: 16px 0 0; font-size: .875rem; color: var(--muted); }
.account strong {
  color: var(--ink); font-weight: 600;
  font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px; display: inline-block; max-width: 100%;
  overflow: hidden; text-overflow: ellipsis; vertical-align: bottom;
}
.foot { margin: 18px 0 0; font-size: 12px; color: var(--muted); }
.foot a { color: var(--accent-700); text-underline-offset: .2em; }
.foot a:focus-visible { box-shadow: 0 0 0 2px var(--accent); outline: none; border-radius: 4px; }
.countdown {
  margin: 10px 0 0; font-size: 12px; color: var(--muted);
  font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-variant-numeric: tabular-nums;
}
[data-hidden="true"] { display: none; }
@keyframes page-in { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: none; } }
@keyframes page-fade { from { opacity: 0; } to { opacity: 1; } }
@keyframes step-in { from { opacity: 0; transform: translateY(6px); } to { opacity: 1; transform: none; } }
@keyframes step-fade { from { opacity: 0; } to { opacity: 1; } }
body[data-closing="true"] .panel {
  opacity: 0; transform: translateY(6px);
  transition: opacity 200ms ease, transform 200ms ease;
}
@media (prefers-reduced-motion: reduce) {
  .panel { animation: page-fade 140ms ease; }
  .steps li[data-state="done"],
  .steps li[data-state="active"],
  .steps li[data-state="failed"] { animation: step-fade 140ms ease; }
  body[data-closing="true"] .panel { transform: none; transition: opacity 140ms ease; }
}
</style>
</head>
<body>
<main class="panel" id="panel"
  data-status-url="{{.StatusURL}}"
  data-final="{{if or .Failed .Expired .Connected}}true{{else}}false{{end}}"
  data-connected-title="{{.Wording.ConnectedTitle}}"
  data-connected-body="{{.Wording.ConnectedBody}}"
  data-failed-title="{{.Wording.FailedTitle}}"
  data-expired-title="{{.Wording.ExpiredTitle}}"
  data-expired-body="{{.Wording.ExpiredBody}}"
  data-failure-denied="{{.Wording.FailureDenied}}"
  data-failure-state="{{.Wording.FailureState}}"
  data-failure-timeout="{{.Wording.FailureTimeout}}"
  data-failure-exchange="{{.Wording.FailureExchange}}"
  data-marker-connected="{{.Wording.MarkerConnected}}"
  data-marker-failed="{{.Wording.MarkerFailed}}"
  data-close-hint="{{.Wording.CloseHint}}"
  data-closing-in="{{.Wording.ClosingIn}}"
  data-close-seconds="{{.CloseSeconds}}"
  data-console-url="{{.Wording.ConsoleURL}}"
  data-console-label="{{.Wording.ConsoleLabel}}">
  <svg class="glyph" width="32" height="32" viewBox="0 0 512 512" fill="none" aria-hidden="true"><g stroke="currentColor" stroke-width="32" stroke-linecap="round" fill="none"><path d="M256 91 A165 165 0 0 1 421 256"/><path d="M256 421 A165 165 0 0 1 91 256"/></g><circle cx="256" cy="256" r="64" fill="currentColor"/></svg>
  <p class="provider" title="{{.Provider}}">{{.Provider}}</p>
  <h1 id="title">{{.Heading}}</h1>
  <p id="summary">{{.Summary}}</p>
  <span class="mark" id="mark" data-state="{{if .Connected}}connected{{else if .Failed}}failed{{else}}working{{end}}">{{.Marker}}</span>
  <ol class="steps" id="steps" aria-label="{{.Wording.StepsLabel}}" data-hidden="{{if .Expired}}true{{else}}false{{end}}">
    <li data-step="authorized" data-state="done">{{.Wording.StepAuthorized}}</li>
    <li data-step="exchanging" data-state="{{if .Failed}}failed{{else if .Connected}}done{{else if .Expired}}skipped{{else}}active{{end}}">{{.Wording.StepExchanging}}</li>
    <li data-step="stored" data-state="{{if .Connected}}done{{else if or .Failed .Expired}}skipped{{else}}pending{{end}}">{{.Wording.StepStored}}</li>
  </ol>
  <p class="account" id="account" data-hidden="{{if .Account}}false{{else}}true{{end}}"><span>{{.Wording.AccountLabel}}</span>: <strong id="account-name" title="{{.Account}}">{{.Account}}</strong></p>
  <p class="foot" id="foot">{{.Wording.CloseHint}}{{if or .Failed .Expired}} <a href="{{.Wording.ConsoleURL}}">{{.Wording.ConsoleLabel}}</a>{{end}}</p>
  <p class="countdown" id="countdown" data-hidden="true"></p>
</main>
<script>
(function () {
  var panel = document.getElementById("panel");
  var finished = panel.getAttribute("data-final") === "true";
  // A finished login counts down in the open, so the wait before the tab
  // closes itself is legible rather than a sudden disappearance.
  // The tab fades out before it closes itself, so the end of the
  // countdown reads as motion rather than a sudden disappearance.
  function beginClose() {
    document.body.setAttribute("data-closing", "true");
    setTimeout(function () { window.close(); }, 220);
  }
  function closeSoon() {
    var row = document.getElementById("countdown");
    if (!row) { setTimeout(function () { window.close(); }, 1000); return; }
    var seconds = parseInt(read("close-seconds"), 10);
    if (isNaN(seconds) || seconds < 0) { seconds = 3; }
    var sentence = read("closing-in");
    row.setAttribute("data-hidden", "false");
    function tick() {
      if (seconds <= 0) { beginClose(); return; }
      row.textContent = sentence.replace("__SECONDS__", String(seconds));
      seconds -= 1;
      setTimeout(tick, 1000);
    }
    tick();
  }
  if (finished) { closeSoon(); return; }
  var url = panel.getAttribute("data-status-url");
  if (!url) { return; }
  var title = document.getElementById("title");
  var summary = document.getElementById("summary");
  var mark = document.getElementById("mark");
  var account = document.getElementById("account");
  var accountName = document.getElementById("account-name");
  var steps = document.getElementById("steps");
  var foot = document.getElementById("foot");

  function step(name, state) {
    var item = steps.querySelector('[data-step="' + name + '"]');
    if (!item) { return; }
    item.setAttribute("data-state", state);
    // Restart the step entrance each time a step changes: the computed
    // animation is identical across states, so it only replays when reset.
    item.style.animation = "none";
    void item.offsetWidth;
    item.style.animation = "";
  }
  function read(name) { return panel.getAttribute("data-" + name); }
  function finish(state, text) {
    finished = true;
    mark.setAttribute("data-state", state);
    mark.textContent = state === "connected" ? read("marker-connected") : read("marker-failed");
    summary.textContent = text;
    foot.textContent = read("close-hint") + " ";
    var link = document.createElement("a");
    link.href = read("console-url");
    link.textContent = read("console-label");
    foot.appendChild(link);
    closeSoon();
  }
  function connected(state) {
    step("exchanging", "done");
    step("stored", "done");
    title.textContent = read("connected-title");
    if (state.account) {
      accountName.textContent = state.account;
      account.setAttribute("data-hidden", "false");
    }
    finish("connected", read("connected-body"));
  }
  function failed(state) {
    step("exchanging", "failed");
    step("stored", "skipped");
    title.textContent = read("failed-title");
    finish("failed", read("failure-" + (state.error || "exchange")) || read("failure-exchange"));
  }
  function expired() {
    steps.setAttribute("data-hidden", "true");
    title.textContent = read("expired-title");
    finish("failed", read("expired-body"));
  }
  function poll() {
    if (finished) { return; }
    fetch(url, { headers: { Accept: "application/json" }, cache: "no-store" })
      .then(function (response) {
        if (response.status === 404) { expired(); return null; }
        if (!response.ok) { return null; }
        return response.json();
      })
      .then(function (state) {
        if (finished) { return; }
        if (!state) { setTimeout(poll, 1000); return; }
        if (state.phase === "connected") { connected(state); return; }
        if (state.phase === "failed") { failed(state); return; }
        setTimeout(poll, 700);
      })
      .catch(function () { if (!finished) { setTimeout(poll, 1500); } });
  }
  poll();
})();
</script>
</body>
</html>
`
