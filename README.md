# flyingwhale-core

flyingwhale-core is a shared Go module for FlyingWhale app servers. It holds common packages extracted out of those servers. Server apps import this module rather than duplicating the same code.

| Package | Role |
| --- | --- |
| `buildinfo` | Reads the build revision and modified flag stamped into a binary. |
| `daykey` | Turns a time and a timezone offset into a calendar day key. |
| `httpx` | HTTP primitives shared by FlyingWhale servers: error envelope and code vocabulary, request id / logging / recover middlewares, rate limiting, router with route groups, Accept-Language negotiation, request-id slog handler. |
| `migrate` | SQLite migration runner driven by user_version. |
| `textmatch` | Multilingual keyword matching: word-boundary for alphabetic scripts, substring for CJK. |
| `webhook` | Checks a webhook request against a shared secret. |

This module is available under the MIT license.
