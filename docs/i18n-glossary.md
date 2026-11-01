# Console i18n glossary

Use these canonical concepts when translating console copy. Product and protocol names stay unchanged.

| English | Concept | Translation rule |
|---|---|---|
| connection | A configured upstream instance | Translate the operator-facing noun consistently; do not use the wire term "provider" as a UI synonym. |
| account | One credential in a connection pool | Use the language's ordinary account word, not "credential" unless the UI is specifically about the stored secret. |
| provider | A vendor or template identifier in technical data | Preserve in IDs and protocol names; translate only when prose names the vendor. |
| model | One model identifier a connection serves | Translate the common noun; keep model IDs and vendor names verbatim. |
| route / group | A named alias with ordered members | Prefer the existing local word for a route or group and use it everywhere on Groups, logs, and the inspector. |
| request | One logical client request | Use the ordinary request word; attempts inside it remain attempts. |
| client key | The credential issued to an agent | Translate "key" naturally and keep "client" where it distinguishes the key from admin credentials. |
| usage | Recorded traffic totals | Use the local word for usage; do not translate token identifiers. |
| quota | A provider limit reading | Use the local word for quota or allowance and keep window labels such as 5h/7d verbatim. |
| limit window | One quota period | Translate "window" idiomatically and keep the duration units short and numeric. |
| failover | Moving a request to the next member/account | Use the established local technical term; keep status codes and provider names unchanged. |
| dashboard | The vendor's web console | Translate the generic noun, but preserve vendor names and URLs. |
| report issue | Opening a GitHub bug report | Translate the verb phrase for the button and keep GitHub as the product name. |
