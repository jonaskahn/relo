# Test Fixtures

All fixtures in this directory are either synthetic (hand-crafted for
testing) or captured from real providers and redacted.

## Rules

- No real tokens, API keys, emails, or account IDs.
- Captured fixtures must be redacted: replace real values with
  placeholder strings like `"sk-test-redacted"` or `"test@example.com"`.
- Document the source and redaction method for each fixture.
- Fixtures are versioned alongside the code.

## Directory Structure

| Directory    | Content                            |
| ------------ | ---------------------------------- |
| `config/`    | TOML configuration fixtures        |
| `codec/`     | Canonical type JSON fixtures       |
| `openai/`    | OpenAI Chat and Responses SSE/JSON |
| `anthropic/` | Anthropic Messages SSE/JSON        |
| `google/`    | Google Gemini SSE/JSON             |
| `oauth/`     | OAuth flow mock responses          |
| `storage/`   | Migration and SQL fixtures         |

## Responses, Messages, and Gemini fixtures

These fixtures are synthetic: they were written by hand to match the
documented Responses, Messages, and GenerateContent shapes, so no captured
provider traffic is stored in this repository.

| Fixture                                | Content                                                                  |
| -------------------------------------- | ------------------------------------------------------------------------ |
| `openai/responses_streaming.txt`       | Responses text stream, usage in the terminal event                       |
| `openai/responses_reasoning.txt`       | Responses stream with a reasoning summary and encrypted content          |
| `openai/responses_tool_call.txt`       | Responses stream with two parallel function calls                        |
| `openai/responses_nonstreaming.json`   | A complete response object with reasoning, message, and call items       |
| `openai/responses_request.json`        | A client Responses request with instructions, items, and a tool          |
| `anthropic/messages_streaming.txt`     | Messages text stream with cache usage                                    |
| `anthropic/messages_thinking.txt`      | Messages stream with a thinking signature and a prefixed tool call       |
| `anthropic/messages_nonstreaming.json` | A complete message object                                                |
| `anthropic/messages_request.json`      | A client Messages request with system, image, thinking, and tool results |
| `google/gemini_streaming.txt`          | GenerateContent text stream                                              |
| `google/gemini_thinking.txt`           | GenerateContent stream with a thought summary                            |
| `google/gemini_tool_call.txt`          | GenerateContent stream with a function call                              |
| `google/gemini_safety.txt`             | GenerateContent stream blocked by a safety finish                        |
| `google/gemini_nonstreaming.json`      | A complete GenerateContent response                                      |

## Cloud Code Assist quota fixtures

This fixture is synthetic: it was written by hand to match the documented
Cloud Code Assist quota shape, with no captured account data.

| Fixture                          | Content                                                                                |
| -------------------------------- | -------------------------------------------------------------------------------------- |
| `google/antigravity_models.json` | A Cloud Code Assist `fetchAvailableModels` body with Gem, Cla, and tiered quota blocks |

## Chat Completions fixtures

These fixtures are synthetic: they were written by hand to match the
documented OpenAI Chat Completions shapes, so no captured provider traffic
is stored in this repository.

| Fixture                         | Content                                                         |
| ------------------------------- | --------------------------------------------------------------- |
| `config/valid.toml`             | A complete startup configuration                                |
| `config/unknown_keys.toml`      | Valid configuration plus keys Relo does not know                |
| `codec/canonical_request.json`  | A canonical request with text, image, tool calls, and reasoning |
| `openai/chat_streaming.txt`     | SSE stream for a text completion, usage chunk included          |
| `openai/chat_tool_call.txt`     | SSE stream whose tool call arguments span three chunks          |
| `openai/chat_nonstreaming.json` | A complete chat completion object                               |
| `openai/chat_error.json`        | An OpenAI error body with a redacted key                        |
