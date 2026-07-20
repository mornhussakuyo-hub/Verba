# Verba implementation status

This document tracks implementation against `design.md`. A feature is marked complete only when parsing, semantic checking, code generation, tests, documentation, and an executable example all agree.

## Current milestone

Verba is an executable `0.1.0` compiler that satisfies the explicit MVP support list and all eight acceptance criteria in `design.md`. This does not mean the broader language design is complete: imported source modules, a stable typed IR, richer adapters, and language-server tooling remain future work.

The current work focuses on a typed semantic pipeline before expanding the runtime. Type-aware scopes, function arguments, field access, conditions, returns, optional values, `result` propagation, and typed `match` expressions are implemented and covered by checker tests.

## MVP acceptance evidence

| Criterion | Evidence |
| --- | --- |
| Complete user service builds | `learn/08_user_service` is checked, audited, and built by `scripts/verify.ps1` on Windows and Ubuntu |
| Invalid JSON reports an exact position | `TestInvalidJSON` asserts the Unicode-aware source line and column |
| Missing or extra SQL bindings fail | `TestSQLMissingAndExtraBindings` asserts `SQL2107` and `SQL2103` |
| Optional access requires unwrap | `TestOptionalFieldRequiresUnwrap` asserts `VRB1440` |
| Block mismatches name their declaration | `TestMissingEndProducesDiagnostic` covers record, enum, function, route, nested statement, and argument blocks |
| Formatter is idempotent | `TestSourceIsIdempotentAndPreservesIsland` runs formatting twice; verification checks every project |
| Generated routes handle required outcomes | `TestBuildAndRunHTTPService` executes 200 success, 400 parse failure, 404 not found, and 500 database failure paths |
| Compiler stages have tests | `go test ./...` covers lexer, parser, checker, region scanner, emitter, compiler, CLI, and adapters |

## Compiler pipeline

| Stage | Status | Remaining work |
| --- | --- | --- |
| Source manager | Partial | UTF-8/BOM validation, stable file IDs, byte offsets, and Unicode line maps work; full spans and imported module sources remain |
| Region scanner | Implemented | Core spans, exact island terminators, byte-preserved raw content, and missing-terminator diagnostics are active in parsing and formatting |
| Core lexer | Partial | Keywords, identifiers, numeric syntax, newlines, controlled literals, and island exclusion work; parser token consumption and broader recovery remain |
| Parser | Partial | Uses scanned island regions and lexer diagnostics; typed route inputs, richer literals, token-driven parsing, and broader recovery remain |
| Name resolver | Partial | Module identity, manifest dependencies, `use` capabilities, typed local scopes, and forward declarations work; imported symbols, case ambiguity, and references remain |
| Type checker | In progress | Typed SQL bindings/results and contextual database errors work; complete builtin signatures and control-flow joins remain |
| Island registry | Partial | PostgreSQL schema snapshots, typed single-table SQL, compiled regexes, and checked HTML/text slots exist; richer SQL/HTML parsers remain |
| Typed IR | Missing | Stable lowered representation between checking and emission |
| Go emitter | Partial | Typed HTTP result boundaries, PostgreSQL queries/transactions, exact decimal SQL/JSON, contextual database failures, regexes, and escaped templates work; full builtins remain |
| Build driver | Partial | Trimmed builds and isolated pinned pgx resolution work; cache, build metadata, and source-mapped backend errors remain |
| Language server | Missing | Diagnostics, navigation, completion, rename, nested island tooling |

## Language and runtime

| Area | Implemented | Remaining work |
| --- | --- | --- |
| Declarations | module, use, record, enum, function, route, embed, strict TOML project manifest | imported source modules and lock files |
| Statements | let, var, set, call, if/else, match/case, for, while, return, respond, executable PostgreSQL transactions | nested transactions and savepoints |
| Types | scalar names, optional, list, map, result, records, enums, contextual numeric constants, exact decimal runtime | explicit conversion APIs |
| Expressions | atoms, controlled text/url/path, call, get, equality | complete builtin set and typed conversion functions |
| HTTP | generated `net/http` routes, path values, body/headers/context bindings, typed route results, and deterministic application-error mapping | typed query/header decoding and explicit per-route mapping metadata |
| JSON | island syntax validation, typed `result` decoding errors, generated encoding | schema options and typed constants |
| SQL | PostgreSQL schema snapshots, named `$n` rewriting, typed bindings/rows, exec/one/optional/many, pgx driver, rollback/commit | joins, subqueries, migrations, live database integration suite |
| HTML | exact template slot checking, generated renderer, escaped dynamic values | structural parser and explicit trusted HTML model |
| Regex | compile-time validation, precompiled resources, runtime matching helper | richer regex diagnostics and editor integration |
| Capabilities | built-in and explicit capability validation, inferred requirements, dependency usage, text/JSON audit output | runtime enforcement and deployment policy generation |

## Documentation

The repository has a design document, README, and an eleven-chapter Chinese beginner tutorial under `docs/tutorial/`. Eight runnable projects under `learn/` are checked and built independently by CI.

## Release gates

- `go test -count=1 ./...`
- `go vet ./...`
- `verba fmt --check` for every example and tutorial project
- `verba check` for every example and tutorial project independently
- executable HTTP smoke tests on Windows and Linux
- deterministic generated Go for identical inputs
- all repository files tracked, CI green, and release notes updated
