---
name: nats-docs
description: Use when answering questions about NATS, JetStream, nats-server configuration, NATS CLI, NATS clients, security, clustering, gateways, leaf nodes, or when searching/retrieving NATS documentation with the standalone nats-docs CLI.
---

# NATS Docs

Use the standalone `nats-docs` CLI as the primary documentation source. It is MCP-independent and embeds a NATS docs snapshot, so it can run as a single executable.

## Workflow

1. Check the index when needed:
   ```bash
   nats-docs doctor
   ```

2. Search for relevant pages:
   ```bash
   nats-docs search "<query>" --limit 5
   ```

3. Retrieve the strongest pages by document ID:
   ```bash
   nats-docs get <doc-id>
   ```

4. Use JSON output when parsing results programmatically:
   ```bash
   nats-docs search "<query>" --limit 5 --json
   nats-docs get <doc-id> --json
   ```

5. Cite the returned document title, ID, and URL in answers when facts come from the docs.

## Source Checkout

When operating inside this repository before installing the binary, invoke the same CLI through Go:

```bash
go run ./cmd/nats-docs-cli search "<query>" --limit 5
go run ./cmd/nats-docs-cli get <doc-id>
```

## Snapshot Awareness

The embedded archive is a local snapshot. For date-sensitive questions such as latest releases, current compatibility, or recently changed behavior, verify against a current upstream source and state when the CLI result comes from the embedded snapshot.
