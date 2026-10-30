# Release Notes — v0.2.0

Covers everything since v0.1.0 (452 commits, 2026-10-15 → 2026-10-30).

🎉 **Free connections are now opt-in**
- 🔒 Off by default; explicit consent is required and enforced server-side in routing
- 🙈 Free gateways stay hidden and renamed until enabled (setting lives under **Others**)
- 🎛️ Toggle in the add picker, per-connection overrides, free-aware review controls

📊 **Dashboard & usage**
- 🌌 New routing universe and route tunnel, attention band for accounts that need you
- 💸 Spend cards with costliest connections, pricing labels, and a dedicated free-lanes card
- 🗓️ Yearly activity calendar with figures (and a snake game 🐍)
- 📈 Grouped usage lists, insight/share cards, token counts abbreviated above 1k

🪵 **Logs**
- 🧾 Request summaries, KPIs, collapsed repeated lines, detail in a drawer
- ✨ Live rows animate in; paging, filters, and scroll position survive refreshes
- 🎭 Impersonate a captured request, tool turns included, and replay it

🔌 **Providers & accounts**
- ➕ New: OpenDesign, Command Code, DeepSeek account import
- 🏷️ Connections classified as free / plan / pay-as-you-go, with billing chips everywhere
- 🧩 Billing lanes and smarter member suggestions for groups
- 🧠 Provider-defined capabilities, limits, and vision read from live listings and models.dev (with an offline seed)

🤖 **Coding clients & integrations**
- 🔄 Reload OpenCode without a restart; clear reasons when an agent needs repair
- 🧪 Thinking/reasoning support for Pi, Oh My Pi, and toggle-only models
- 🚫 OpenCode V1 no longer offered for new setups
- 🖱️ Cursor wire: heartbeats, inline images, native exec handoff, tool-call replay fixes

📦 **Install & updates**
- ⬆️ Release catalog, changelog in the update dialog, beta channel, per-install upgrade commands
- 🧰 npm: start-at-login, unattended install, safer Relo.app detection, binary pinned to package version
- 🪟 Windows: GUI binary, shim handling, stdin fixes
- 🐧 Linux setup packages published; script installs detected on uninstall

🐛 **Reliability**
- 🛠️ Tool-call handling: one result per call, JSON-object arguments, text-written calls recovered
- 📏 Oversized output budgets fit to context; clearer per-client context-overflow errors
- 🧯 Streamed failures terminate cleanly; startup ordering races fixed
- 🔁 Catalog refresh switches off models no longer listed; snapshot reloads serialized

🎨 **Console polish**
- 🌍 Local-timezone dates, 20-slot 2026 accent board, drawer dialogs, phone-friendly layouts
- 🗣️ 32 i18n updates across new screens

🧹 **Under the hood**
- 🧪 50 test commits and expanded coverage, 14 refactors, SvelteKit 3 migration
