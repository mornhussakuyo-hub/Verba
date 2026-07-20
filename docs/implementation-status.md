# Verba implementation status

This document tracks implementation against `design.md`. A feature is marked complete only when parsing, semantic checking, code generation, tests, documentation, and an executable example all agree.

## Current milestone

Verba is an executable `0.1.0` vertical slice. It can parse, format, check, generate Go, and build small HTTP services. It is not yet the complete MVP described by the design document.

The current work focuses on a typed semantic pipeline before expanding the runtime. Type-aware scopes, function arguments, field access, conditions, returns, optional values, `result` propagation, and typed `match` expressions are implemented and covered by checker tests.

## Compiler pipeline

| Stage | Status | Remaining work |
| --- | --- | --- |
| Source manager | Partial | UTF-8/BOM validation, stable file IDs, byte offsets, and Unicode line maps work; full spans and imported module sources remain |
| Region scanner | Partial | Dedicated core/literal/island regions and byte-accurate island spans |
| Core lexer | Missing | Tokens, keyword classification, numeric validation, recovery |
| Parser | Partial | Typed route inputs, richer literals, and broader recovery coverage |
| Name resolver | Partial | Typed local scopes exist; modules, imports, case ambiguity, references remain |
| Type checker | In progress | Numeric constants, complete builtin signatures, control-flow joins, SQL result types |
| Island registry | Partial | JSON and SQL binding checks, compiled regexes, and checked HTML/text slots exist; SQL schema metadata and richer island parsers remain |
| Typed IR | Missing | Stable lowered representation between checking and emission |
| Go emitter | Partial | HTTP, typed JSON/UUID failures, regexes, and escaped templates work; SQL, application error mapping, and full builtins remain |
| Build driver | Partial | Trimmed reproducible builds work; cache, build metadata, and source-mapped backend errors remain |
| Language server | Missing | Diagnostics, navigation, completion, rename, nested island tooling |

## Language and runtime

| Area | Implemented | Remaining work |
| --- | --- | --- |
| Declarations | module, use, record, enum, function, route, embed, strict TOML project manifest | imported source modules and lock files |
| Statements | let, var, set, call, if/else, match/case, for, while, return, respond, transaction parsing | executable transactions |
| Types | scalar names, optional, list, map, result, records, enums | strict numeric constants, decimal runtime, conversion APIs |
| Expressions | atoms, controlled text/url/path, call, get, equality | complete builtin set and typed conversion functions |
| HTTP | generated `net/http` routes, path values, body/headers/context bindings, route error boundary | typed query/header decoding and declared application-error mapping |
| JSON | island syntax validation, typed `result` decoding errors, generated encoding | schema options and typed constants |
| SQL | named parameter extraction and exact binding checks | dialect parser, schema snapshot, drivers, rows, transactions |
| HTML | exact template slot checking, generated renderer, escaped dynamic values | structural parser and explicit trusted HTML model |
| Regex | compile-time validation, precompiled resources, runtime matching helper | richer regex diagnostics and editor integration |
| Capabilities | `use` is parsed | capability validation, audit output, runtime enforcement |

## Documentation

The repository has a design document, README, and an eight-chapter Chinese beginner tutorial under `docs/tutorial/`. Five runnable projects under `learn/` are checked and built independently by CI.

## Release gates

- `go test -count=1 ./...`
- `go vet ./...`
- `verba fmt --check` for every example and tutorial project
- `verba check` for every example and tutorial project independently
- executable HTTP smoke tests on Windows and Linux
- deterministic generated Go for identical inputs
- all repository files tracked, CI green, and release notes updated
