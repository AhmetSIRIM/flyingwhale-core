# flyingwhale-core

flyingwhale-core is a shared Go module for FlyingWhale app servers. It holds common packages extracted out of those servers. Several server apps import this module instead of duplicating the same code.

| Package | Role |
| --- | --- |
| `buildinfo` | Reads the build revision and modified flag stamped into a binary. |
| `daykey` | Turns a time and a timezone offset into a calendar day key. |
| `webhook` | Checks a webhook request against a shared secret. |

This module is available under the MIT license.
