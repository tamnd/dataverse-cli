---
title: "Configuration"
description: "Environment variables, defaults, and the data directory."
weight: 20
---

dataverse needs almost no configuration: it runs anonymously against public
data out of the box. The settings below let you tune politeness and storage.

## Defaults

| Setting | Default | Flag |
|---|---|---|
| Requests | paced and retried on 429/5xx | `--rate`, `--retries` |
| Per-request timeout | 30s | `--timeout` |
| On-disk cache | under the data directory | `--no-cache` to bypass |

## The data directory

Caches and any record store live under one data directory, chosen in this order:

1. `--data-dir`
2. `DATAVERSE_DATA_DIR`
3. `$XDG_DATA_HOME/dataverse`
4. `~/.local/share/dataverse`

## Environment variables

Every flag has an environment fallback, prefixed `DATAVERSE_` in
upper case with dashes as underscores. For example:

```bash
export DATAVERSE_RATE=1s        # same as --rate 1s
export DATAVERSE_DATA_DIR=~/data/dataverse
```

Flags win over environment variables, which win over the built-in defaults.

## Sending records to a store

`--db` tees every emitted record into a store as a side effect of reading, so a
session fills a local database without a separate import step:

```bash
dataverse page <path> --db out.db        # SQLite file
dataverse page <path> --db 'postgres://...'
```
