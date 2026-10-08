# Glossary

These words mean one thing everywhere: in Go, in SQLite, in the API, in the
console, and in this document.

| Term         | Meaning                                                                                      |
|--------------|----------------------------------------------------------------------------------------------|
| Template     | A curated or models.dev option an operator adds a connection from                            |
| Connection   | A configured upstream instance, created from a template or from custom input                 |
| Account      | One credential in a connection's pool: an API key, AWS keys, a service account, or a sign-in |
| Model        | A model one connection serves                                                                |
| Route        | A named model alias with ordered or weighted members                                         |
| Client key   | The credential the data plane issues to an agent                                             |
| Admin token  | The credential that reaches the management API                                               |
| Draft        | An in-memory connection setup session: settings, verification, and an unfinished login       |
| Surface      | One native client endpoint: `chat-completions`, `responses`, or `messages`                   |
| Format       | An upstream wire format                                                                      |
| Model index  | The models.dev dataset and its saved copy                                                    |
| Quota window | A usage limit period reported for one account                                                |

"Provider" survives only in wire names and in the OAuth adapter, where a
flow's provider identifier is its template identifier.
