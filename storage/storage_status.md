# Storage Operations Status

Last Reviewed: 2026-03-19
Audience: Engineering, Procurement, Security Vetting, CISO
Scope: Local persistence and file system operations in the storage boundary

## Executive Summary

This document tracks storage-side operations in git-lrc as an auditable inventory for enterprise due diligence.

- Storage boundary: local file system and local SQLite only (no outbound API calls in this package).
- Modes represented: file, db.
- Operation count tracked: 47 operations.
- Severity distribution: High 10, Medium 15, Low 22.
- Primary sensitive data in scope: API keys and connector state in config, review metadata in SQLite, hook scripts and metadata, update lock/state metadata.
- Highest-risk operation classes: credential file read/write, recursive deletion, permission changes, direct SQL execution wrappers.
- Primary compensating controls already present: atomic writes for critical files, SQLite WAL mode and busy timeout, explicit chmod utility usage, typed wrapper functions and contextual error wrapping.
- High-priority updates added in this review: mode-specific permission tests, backward-compatible schema version marker note, optional dry-run/logged branch delete API, optional confirmation-gated full delete API, review-session SQL mutation routing through ExecSQL, and pending-update integrity hash validation with legacy compatibility.
- Current diff note: hook install reliability fix now handles wrapped file-not-found errors and normalizes hooksPath values before install/uninstall orchestration; no new storage operation APIs added in this increment.
- Current diff note: uninstall shell source-line cleanup now preserves original LF/CRLF style and replaces string literals with named constants for maintainability.
- Current diff note: RemoveFileIfExists now avoids stat-then-remove TOCTOU on non-dry-run path, shell-line splitting now respects detected line ending delimiter, and fish managed-marker string is constantized.
- Current diff note: story phase-1 transcript extraction adds VS Code/Copilot discovery and transcript read wrappers in storage for filesystem-bound chat history discovery.

## Severity Rubric

- High: operation can expose credentials, alter integrity-critical state, perform broad deletion, change permissions, or mutate persistent review/session records.
- Medium: operation mutates non-critical persisted state or reads/writes operational data that can impact behavior but is lower sensitivity.
- Low: operation creates directories, reads low-sensitivity input, or performs cleanup with limited impact.

## Risk Acknowledgement Rules

- Every operation row must state known risk and compensation status.
- High-severity rows must include explicit compensation or explicit suggestion marker.
- Suggestion marker format: Suggestion: <compensation>
- Acceptable residual risk must be called out when controls are considered sufficient.

## Inventory: Config And Credential I/O

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ReadConfigFile | file | TOML config bytes including API key and connector state | Load CLI configuration from ~/.lrc.toml | High | Credential disclosure risk if file is too permissive | Compensated by strict mode enforcement via chmod path; residual risk acceptable with 0600 policy | [storage/config_io.go](config_io.go#L9) |
| WriteFileAtomically | file | Generic file bytes (used for durable state/config writes) | Persist file content using temp-and-rename pattern | High | Integrity risk from partial/truncated writes | Compensated by atomic temp-then-rename; residual risk acceptable for local FS assumptions | [storage/files.go](files.go#L29) |
| Chmod | file | File mode bits (0600/0755 style permissions) | Enforce permission model on config and scripts | High | Misconfiguration risk if wrong mode is applied | Compensated by centralized wrapper plus mode-specific tests for secret and executable paths; residual risk acceptable | [storage/files.go](files.go#L119) |
| MkdirAll | file | Directory paths | Create required storage folders safely | Low | Low risk of directory sprawl/path misuse | Compensated by controlled internal callsites; acceptable risk | [storage/files.go](files.go#L87) |

## Inventory: Review And Attestation Database

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| OpenAttestationReviewDB | db | SQLite handle for review sessions | Open persistent review DB with WAL behavior | High | DB integrity/availability risk on contention or corruption | Compensated by WAL mode and busy timeout in open path; residual risk acceptable | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L37) |
| InitializeAttestationReviewSchema | db | SQL DDL for review_sessions | Create required schema for branch review tracking | High | Schema drift risk impacts audit evidence correctness | Compensated by centralized init path with explicit schema version marker note and backward-compatible marker check; missing marker remains non-fatal for legacy schemas | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L51) |
| InsertAttestationReviewSessionRow | db | branch, tree_hash, action, diff files, review_id, timestamp | Record review session evidence | High | Audit trail tampering/inconsistency risk | Compensated by single writer path and typed insert wrapper; residual risk acceptable | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L66) |
| QueryAttestationReviewSessionCountByBranch | db | Aggregate count | Report branch review volume | Medium | Reporting accuracy risk if stale or partial state | Compensated by direct DB query on canonical table; acceptable risk | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L81) |
| QueryAttestationReviewedSessionsByBranch | db | Ordered review session rows | Retrieve historical review evidence by branch | Medium | Evidence retrieval ordering/completeness risk | Compensated by explicit ordering query; acceptable risk | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L93) |
| DeleteAttestationReviewSessionsByBranch | db | Branch-scoped review rows | Purge branch review history | High | Data loss and forensic gap risk | Compensated by scoped delete API plus optional (opt-in) dry-run and audit logging options; default path has no additional logging overhead | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L111) |
| DeleteAllAttestationReviewSessions | db | Entire review_sessions table | Administrative full wipe of review history | High | High-impact irreversible evidence loss risk | Partially compensated by optional caller confirmation gate API with legacy delete path preserved for backward compatibility; residual risk remains high unless callers adopt confirmation policy | [storage/attestation_review_db_io.go](attestation_review_db_io.go#L148) |
| OpenSQLite | db | Generic SQLite connection and PRAGMA state | Standardized DB opener utility | High | Broad DB behavior risk if PRAGMA policy regresses | Compensated by centralized opener policy and wrapped errors; residual risk acceptable | [storage/files.go](files.go#L145) |
| ExecSQL | db | SQL statement plus args | Execute SQL with wrapped errors | High | SQL misuse risk from broad execution capability | Compensated by using ExecSQL for all review_sessions mutation paths in storage (schema init, insert, branch delete, full delete); direct db.Exec remains only inside ExecSQL wrapper | [storage/files.go](files.go#L161) |

## Inventory: Hook Lifecycle Storage

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| EnsureHooksPathDir | file | .git/hooks directory path | Ensure Git hooks root exists | Low | Low risk from directory creation side effects | Compensated by predictable git path target; acceptable risk | [storage/hook_io.go](hook_io.go#L21) |
| EnsureManagedHooksDir | file | .git/hooks/lrc directory | Provision lrc-managed hook directory | Low | Low risk of local repo state mutation | Compensated by dedicated managed subdirectory; acceptable risk | [storage/hook_io.go](hook_io.go#L13) |
| EnsureHooksBackupDir | file | .git/hooks/.lrc_backups directory | Provision backup location for displaced hooks | Low | Low risk of backup directory growth | Compensated by scoped location and cleanup routines; acceptable risk | [storage/hook_io.go](hook_io.go#L29) |
| EnsureRepoLRCStateDir | file | .git/lrc directory | Provision repo-local lrc state storage | Low | Low risk from creating local state directory | Compensated by deterministic repo-local path; acceptable risk | [storage/hook_io.go](hook_io.go#L37) |
| ReadHookFile | file | Hook script bytes | Inspect existing hook script content | Medium | Medium risk of parsing/handling untrusted local hook content | Compensated by read-only behavior and bounded scope; acceptable risk | [storage/hook_io.go](hook_io.go#L45) |
| ReadHookMetaFile | file | Hook metadata JSON bytes | Load metadata used for hook management | Medium | Metadata poisoning risk from local tampering | Partially compensated by controlled metadata format; Suggestion: add schema validation check | [storage/hook_io.go](hook_io.go#L54) |
| RemoveHookMetaFile | file | Hook metadata file | Cleanup metadata during uninstall/reset | Low | Low risk of leaving stale metadata if delete fails | Compensated by cleanup intent and low criticality; acceptable risk | [storage/hook_io.go](hook_io.go#L64) |
| RemoveManagedHooksDir | file | .git/hooks/lrc directory tree | Remove lrc-managed hooks during uninstall | Medium | Medium risk of accidental broad deletion | Compensated by fixed managed directory target; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L9) |
| RemoveHooksBackupDir | file | .git/hooks/.lrc_backups directory tree | Remove hook backup tree during cleanup | Medium | Medium risk of losing restoration artifacts | Partially compensated by explicit backup path boundary; Suggestion: optional backup retention switch | [storage/file_delete_io.go](file_delete_io.go#L18) |
| RemoveHookBackupFile | file | Individual backup script | Remove stale backup files | Medium | Medium risk of removing needed backup artifact | Compensated by explicit file-target deletion path; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L59) |
| RemoveHookScriptFile | file | Individual hook script | Remove managed hook scripts | Medium | Medium risk of removing expected hook behavior | Compensated by managed-hook ownership model; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L67) |
| RemoveRepoHooksDisabledMarker | file | Marker file in repo state | Clear local marker used to disable hook path behavior | Low | Low risk of local behavior drift if marker handling fails | Compensated by simple marker semantics; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L75) |
| RemoveDirIfEmpty | file | Directory path | Defensive cleanup only if empty | Low | Low risk of unintended deletion | Compensated by emptiness check guard; acceptable risk | [storage/hook_io.go](hook_io.go#L73) |

## Inventory: Review Inputs, Temporary Files, And Cleanup

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ReadInitialMessageFile | file | Initial review prompt text | Load user-provided message for review request flow | Low | Low sensitivity text read risk | Compensated by local read-only operation; acceptable risk | [storage/review_input_io.go](review_input_io.go#L9) |
| ReadDiffFile | file | Diff bytes | Load diff payload before review submission | Medium | Medium confidentiality risk for code diff content | Partially compensated by local-only read path; Suggestion: document max retention expectations | [storage/review_input_io.go](review_input_io.go#L18) |
| CreateTempReviewHTMLFile | file | Temporary HTML file handle/content | Create temporary location for rendered review output | Low | Low to medium leakage risk if temp files linger | Partially compensated by cleanup operations; Suggestion: enforce cleanup-on-exit where possible | [storage/review_input_io.go](review_input_io.go#L28) |
| RemoveTempHTMLFile | file | Temporary HTML file | Cleanup rendered review temp artifact | Low | Low risk if cleanup fails and file remains | Compensated by dedicated cleanup call; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L27) |
| RemoveSetupLogFile | file | Setup log file | Cleanup setup diagnostics artifact | Low | Low risk if diagnostic artifact persists | Compensated by cleanup path and low sensitivity default; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L35) |
| RemoveReauthLogFile | file | Re-auth log file | Cleanup auth diagnostics artifact | Low | Low to medium risk if logs capture sensitive context | Partially compensated by explicit deletion utility; Suggestion: verify log redaction policy | [storage/file_delete_io.go](file_delete_io.go#L43) |
| RemoveEditorWrapperScript | file | Temporary shell wrapper script | Cleanup git editor wrapper used during review flow | Low | Low risk from temporary script persistence | Compensated by explicit cleanup path; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L83) |
| RemoveEditorBackupStateFile | file | Editor backup state JSON | Cleanup backup state artifact | Low | Low risk from stale local state file | Compensated by explicit cleanup path; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L91) |
| RemoveCommitMessageOverrideFile | file | Commit message override file | Cleanup pending override from commit pipeline | Low | Low risk from stale override file affecting UX | Compensated by cleanup routine; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L99) |
| RemoveCommitPushRequestFile | file | Push-request marker file | Cleanup post-commit push marker | Low | Low risk of stale marker state | Compensated by simple marker cleanup semantics; acceptable risk | [storage/file_delete_io.go](file_delete_io.go#L107) |
| RemoveFileIfExists | file | Generic target file path | Remove installer artifacts with dry-run support | Medium | Medium risk of deleting user-local files when wrong target is supplied | Compensated by explicit callsite-owned target lists and existence checks before removal; residual risk acceptable | [storage/uninstall_io.go](uninstall_io.go#L22) |
| RemoveLRCInstallerShellSourceLines | file | Shell rc file content | Remove installer-owned lrc startup snippets | Medium | Medium risk of modifying user shell startup files | Compensated by marker-based line matching constrained to installer signatures; residual risk acceptable | [storage/uninstall_io.go](uninstall_io.go#L43) |
| RemoveManagedFishLRCConfig | file | fish conf.d file content/path | Remove installer-managed fish integration file | Low | Low risk because removal is gated on managed-file marker content | Compensated by strict marker check before delete; acceptable risk | [storage/uninstall_io.go](uninstall_io.go#L99) |
| RemoveDirIfEmptyIfExists | file | Directory path | Remove now-empty installer directory without recursion | Low | Low risk of unintended deletion because operation is empty-dir constrained | Compensated by explicit emptiness check before removal; acceptable risk | [storage/uninstall_io.go](uninstall_io.go#L123) |

## Inventory: Story Transcript Discovery And Extraction

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PathExists | file | Candidate local path metadata | Check whether explicit or default VS Code user-data roots exist before discovery | Low | Low risk helper can reveal only local path presence | Compensated by read-only stat semantics and narrow discovery-only use; acceptable risk | [story_vscode_io.go](story_vscode_io.go#L11) |
| ListVSCodeWorkspaceStorageDirs | file | Workspace storage directory names | Enumerate VS Code workspace storage roots under a user-data directory | Low | Low risk of listing local directory names outside requested scope | Compensated by fixed workspaceStorage boundary under user-data root; acceptable risk | [story_vscode_io.go](story_vscode_io.go#L22) |
| ListCopilotTranscriptFiles | file | Copilot transcript file paths | Enumerate available GitHub Copilot transcript JSONL files for a workspace | Medium | Medium confidentiality risk because transcript file names and presence reveal chat activity | Compensated by read-only enumeration limited to provider transcript directory; residual risk acceptable | [story_vscode_io.go](story_vscode_io.go#L43) |
| ReadCopilotTranscriptFile | file | Transcript JSONL bytes containing visible chat content and raw provenance | Load one Copilot transcript for story inspection/export | Medium | Medium confidentiality risk because transcripts may contain prompts, tool calls, and reasoning provenance | Compensated by local read-only path with explicit caller-controlled export flow; residual risk acceptable for phase-1 local extraction | [story_vscode_io.go](story_vscode_io.go#L67) |

## Inventory: Self-Update State And Lock Files

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ReadPendingUpdateStateBytes | file | Update state JSON (version, binary path, timestamp, integrity hash) | Read staged update metadata for upgrade flow | Medium | Medium integrity risk if state is tampered locally | Compensated by integrity hash verification when present plus legacy-state compatibility when absent; residual risk acceptable for local tamper-evidence model | [storage/file_read_io.go](file_read_io.go#L9) |
| ReadUpdateLockMetadataBytes | file | Lock metadata JSON (pid, uid, command, version) | Read lock metadata for update concurrency awareness | Medium | Medium risk if lock semantics are informational only | Partially compensated by visibility into lock owner; Suggestion: document/enforce lock semantics in caller | [storage/file_read_io.go](file_read_io.go#L18) |
| OpenFileForRead | file | File handle in read mode | Controlled read access helper | Low | Low risk helper abstraction | Compensated by narrow read-only intent; acceptable risk | [storage/file_read_io.go](file_read_io.go#L27) |

## Control Signals For Security Review

- Boundary separation: storage package centralizes local persistence operations, simplifying audit scope.
- Atomic persistence available: WriteFileAtomically reduces partial-write risk for critical state.
- Permission control available: Chmod utility enables restricted modes for secrets and executable scripts.
- DB durability and contention controls: SQLite WAL mode and busy timeout are configured through opener utilities.
- Error context wrapping: most storage wrappers include operation-specific context in error paths.

## Known Gaps And Follow-Ups

| Gap | Why It Matters | Follow-Up |
| --- | --- | --- |
| Duplicate query paths for review sessions exist across storage files | Duplicate logic can drift and confuse auditors | Identify canonical path and deprecate duplicate wrapper set |
| Explicit schema migration workflow for review DB is not documented in this package | Harder to reason about schema evolution controls | Schema version marker note is now present; next step is a migration policy reference in docs |
| Lock metadata read path is documented, lock enforcement semantics are not explicit here | Concurrency guarantees for update flow may be unclear | Document enforcement owner and decision path in update docs |
| Some cleanup calls intentionally ignore non-critical errors | May reduce forensic clarity in uninstall/cleanup incidents | Document which cleanup failures are intentionally non-fatal |

## Review Cadence

- Update this file when any function is added/removed/renamed in the storage package.
- Re-evaluate severity when data sensitivity changes or when operation side effects change.
- Security review trigger: any new High operation or any change touching credentials, permissions, deletion scope, or DB mutation behavior.
