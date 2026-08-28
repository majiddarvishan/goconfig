# Core Contracts

This document records the decisions made in Phase 2. It defines the target behavior for the refactoring phases while preserving the supported v1 surface where practical.

The transaction, path, snapshot, numeric, schema-compilation, observer, and public Source portions of this contract are implemented as of Phase 4.

## Compatibility policy

- The current public API remains source-compatible throughout the v1 line unless retaining it would preserve a correctness or security defect.
- Idiomatic replacements may be added in v1. Existing names then remain as documented deprecated adapters.
- Removal or semantic changes that cannot be represented by a compatibility adapter are deferred to v2.
- Existing mutation paths containing ordinary object keys and decimal array indexes remain valid because they are also valid JSON Pointers.
- Incorrect behavior, data races, pointer exposure, silent numeric truncation, and undocumented ordering are defects, not compatibility guarantees.

## Canonical paths

- Internal mutation and registration paths use RFC 6901 JSON Pointer semantics.
- The empty string identifies the document root.
- `/` identifies an object member whose key is the empty string; it is not the canonical root path.
- `~1` represents `/` inside a segment and `~0` represents `~`.
- Empty segments are preserved.
- Array indexes are canonical base-10 non-negative integers. `0` is valid; leading zeroes on multi-digit indexes are rejected by the future resolver.
- Mutation paths address existing containers or values according to the selected operation; the Phase 3 resolver will define operation-specific bounds.
- The existing `Query("/")` root behavior remains a v1 compatibility rule until query adopts a distinct canonical API.

Executable parsing and escaping cases are covered by `path_test.go`. Phase 3 will route mutation traversal through these primitives.

## Schema contract

- A Manager requires a non-empty, syntactically valid JSON Schema in v1.
- The initial configuration must satisfy the schema before Manager construction succeeds.
- Every candidate mutation must satisfy the same compiled schema before persistence.
- Schema compilation should happen once during Manager construction in a later phase.
- Optional schema validation, if needed, will be introduced through an explicit option rather than treating an empty schema ambiguously.

## Mutation and callback ordering

The target transaction order is:

1. Read one committed snapshot and check the expected version.
2. Apply the requested operation to a private candidate.
3. Run the compiled JSON Schema validator.
4. Run custom pre-commit validators.
5. Run external validation when configured.
6. Persist the candidate.
7. Atomically publish the snapshot, version, paths, and history event.
8. Invoke post-commit observers with immutable change data after releasing Manager locks.

Existing `OnInsert`, `OnRemove`, and `OnReplace` callbacks retain their v1 veto behavior: returning an error rejects the candidate. They are compatibility pre-commit hooks and must receive private immutable/copy data; they must never observe a temporarily published state. A new observer API may be introduced for non-vetoable after-commit notifications.

User callbacks must not execute while a Manager lock is held. If the committed version changes while a callback or remote validator runs, commit must fail with a typed conflict or safely restart from a fresh snapshot; it must not overwrite the newer change.

## External validation

- External validation is disabled when no service is configured.
- When configured, the default policy is fail-closed: timeout, transport error, malformed response, or explicit rejection aborts the mutation.
- Callers may later opt into a documented fail-open policy explicitly; fail-open is never implicit.
- The request uses the caller/operation context and a bounded timeout.
- External validation runs against the complete candidate configuration, not only the changed node.

## Ordering

- Serialized configuration produced by Source should preserve object key order when the selected representation supports it.
- JSON object order is not part of semantic equality.
- Query results derived from object traversal must be deterministic; the target order is lexical by decoded key, independent of map iteration and source insertion order.
- Array order is always preserved.

## Errors and versions

- Public operations should return typed errors that support `errors.Is` or `errors.As` for invalid input, missing path, type mismatch, bounds, validation, version conflict, persistence, and external service failure.
- Error strings are diagnostic details and are not stable API contracts.
- A successful mutation increments the version exactly once.
- Failed mutations do not change the visible configuration, version, paths, or history.
- Expected-version comparison is part of the atomic commit operation.

## Source of truth and snapshots

- Manager has one canonical committed configuration snapshot.
- Node and JSON/HTTP representations are derived views of that snapshot, not independently mutable stores.
- Public read APIs return immutable values or independent copies.
- Source persistence receives a private candidate and must not retain caller-owned mutable references.
