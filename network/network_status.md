# Network Operations Status

Last Reviewed: 2026-03-19
Audience: Engineering, Procurement, Security Vetting, CISO
Scope: Outbound and proxied network operations in the network boundary

## Executive Summary

This document tracks network-side operations in git-lrc as an auditable inventory for enterprise due diligence.

- Network boundary: outbound HTTP API operations and response handling in network package.
- Modes represented: api.
- Operation count tracked: 18 operations.
- Severity distribution: High 9, Medium 7, Low 2.
- Current diff note: uninstall implementation moved file I/O into storage; no network operation surface change.
- Current diff note: hooks install reliability fix (wrapped not-exist handling and hooksPath normalization) changed only hooks/appcore logic; no network operation surface change.
- Current diff note: uninstall shell cleanup robustness update (line-ending preservation and constant extraction) is storage-only; no network operation surface change.
- Current diff note: uninstall storage refinements (TOCTOU-safe remove path and delimiter-aware line splitting) are storage-only; no network operation surface change.
- Current diff note: story phase-1 VS Code Copilot transcript discovery/export is storage-and-appcore only; no network operation surface change.
- Current diff note: story commit attachment, sparse markdown export, and commit-time attestation refresh are local appcore/storage flows only; no outbound network operation surface change.
- Primary sensitive data in scope: API keys, bearer tokens, org-context headers, diff content, connector validation payloads, update manifest metadata, binary download stream.
- Highest-risk operation classes: review submission/polling, setup credential operations, proxy forwarding paths, binary download operations.
- Primary compensating controls already present: explicit auth header usage by flow, URL normalization helpers, host allowlist check for self-update download sources, timeout-based polling with cancellation support.

## Severity Rubric

- High: operation sends/receives sensitive auth material, drives core review workflow, or downloads executable artifacts.
- Medium: operation forwards or requests operational payloads with constrained impact and bounded scope.
- Low: best-effort telemetry or low-impact ancillary calls.

## Risk Acknowledgement Rules

- Every operation row must state known risk and compensation status.
- High-severity rows must include explicit compensation or explicit suggestion marker.
- Suggestion marker format: Suggestion: <compensation>
- Acceptable residual risk must be called out when controls are considered sufficient.

## Inventory: Review Submission And Polling APIs

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ReviewSubmit | api | Base64 diff review payload, review metadata | Submit review request to LiveReview endpoint | High | Confidentiality and integrity risk for source diff payload in transit | Compensated by authenticated API path and bounded endpoint model; residual risk acceptable with TLS assumptions | [network/review_operations.go](review_operations.go#L6) |
| ReviewPoll | api | Review status payload and comments | Poll asynchronous review completion | High | Availability risk from polling loop and timeout behavior | Compensated by timeout-bound polling with cancellation support; residual risk acceptable | [network/review_operations.go](review_operations.go#L16) |
| ReviewTrackCLIUsage | api | Telemetry event payload | Best-effort CLI usage tracking | Low | Low impact telemetry data transmission risk | Compensated by best-effort non-blocking behavior; acceptable risk | [network/review_operations.go](review_operations.go#L11) |

## Inventory: Setup, Auth, And Connector APIs

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SetupEnsureCloudUser | api | Access-token-authenticated identity payload | Ensure cloud user record exists during setup | High | Authentication-context misuse risk | Compensated by bearer token boundary and scoped setup endpoint; residual risk acceptable | [network/setup_operations.go](setup_operations.go#L6) |
| SetupRefreshTokens | api | Refresh token request payload and returned token set | Refresh auth session tokens during setup and re-auth flows | High | Token handling and session integrity risk | Compensated by scoped auth-refresh endpoint and typed request path in setup operations; residual risk acceptable | [network/setup_operations.go](setup_operations.go#L11) |
| SetupCreateAPIKey | api | API key label and returned plaintext API key | Create connector/review API key | High | High sensitivity secret material exposure risk | Compensated by explicit auth flow and setup caller no-log/no-echo handling for create-key error paths; residual risk acceptable | [network/setup_operations.go](setup_operations.go#L16) |
| SetupValidateConnectorKey | api | Provider key and validation request body | Validate AI connector key before persistence | High | Third-party key exposure/handling risk | Compensated by authenticated request path plus connector key redaction in setup error surfaces; residual risk acceptable | [network/setup_operations.go](setup_operations.go#L21) |
| SetupCreateConnector | api | Connector configuration payload | Persist connector configuration via LiveReview API | High | Misconfiguration and sensitive metadata transmission risk | Compensated by bearer auth plus org context boundary; residual risk acceptable | [network/setup_operations.go](setup_operations.go#L26) |

## Inventory: Proxy And Forwarding APIs

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ReviewProxyRequest | api | Generic forwarded payloads with API key auth | Proxy events/webhook-style calls to configured endpoint | Medium | Medium abuse risk due to forwarding flexibility | Partially compensated by API key auth; Suggestion: document allowed method/path policy for deployments | [network/review_operations.go](review_operations.go#L21) |
| ReviewForwardJSONWithBearer | api | JSON body, bearer token, org context | Forward authenticated JSON requests across setup flows | Medium | Medium risk from header/context propagation mistakes | Compensated by explicit bearer plus org-context request construction; acceptable risk | [network/review_operations.go](review_operations.go#L27) |

## Inventory: Self-Update Network Operations

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SelfUpdateFetchManifest | api | Update manifest metadata and checksum references | Retrieve global update manifest | Medium | Medium integrity risk if manifest source is untrusted | Compensated by controlled update source design and follow-on verification path; acceptable risk | [network/selfupdate_operations.go](selfupdate_operations.go#L32) |
| SelfUpdateFetchReleaseManifest | api | Platform-specific release manifest | Retrieve release details for current target platform | Medium | Medium integrity risk from release metadata tampering | Compensated by expected-host model and verification pipeline assumptions; acceptable risk | [network/selfupdate_operations.go](selfupdate_operations.go#L37) |
| SelfUpdateDownloadBinaryTo | api | Binary stream bytes for executable update artifact | Download release binary to target path | High | High integrity and supply-chain risk for executable download | Compensated by source host validation and SHA256 verification during staging in internal/selfupdate downloadVersionBinaryFromManifest; residual risk acceptable | [network/selfupdate_operations.go](selfupdate_operations.go#L45) |

## Inventory: HTTP Transport And Error Handling Utilities

| Operation | Mode | Data Handled | Purpose | Severity | Risk Acknowledgement | Compensation Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Client.DoJSON | api | Request/response JSON payload bytes | Standard JSON HTTP call wrapper | Medium | Medium risk from broad transport usage and status-handling variance | Compensated by centralized transport wrapper with timeout controls; acceptable risk | [network/http_client.go](http_client.go#L43) |
| Client.Do | api | Raw HTTP request/response bytes | Generic HTTP call wrapper for non-JSON/raw workflows | Medium | Medium risk from raw payload handling flexibility | Partially compensated by shared client boundary; Suggestion: document callsite expectations for raw bodies | [network/http_client.go](http_client.go#L89) |
| SetupEnsureCloudUserURL | api | Base URL plus endpoint normalization inputs | Normalize endpoint composition and reduce path ambiguity | Medium | Medium risk if normalization logic diverges from endpoint assumptions | Compensated by centralized URL builder utility; acceptable risk | [network/endpoints.go](endpoints.go#L13) |
| PollReview | api | Review IDs, status payloads, timeout state | Timeout-bounded polling orchestration in review runtime | High | High availability/latency risk if review service is degraded | Compensated by bounded timeout and interval controls; residual risk acceptable | [internal/reviewapi/helpers.go](../internal/reviewapi/helpers.go#L181) |
| formatJSONParseError | api | Response body text for parse diagnostics | Improve operator diagnostics when endpoint/port mismatches occur | Low | Low risk diagnostic utility behavior | Compensated by safer error interpretation path; acceptable risk | [internal/reviewapi/helpers.go](../internal/reviewapi/helpers.go#L109) |

## Control Signals For Security Review

- Auth separation by flow: API-key and bearer-token paths are distinct and explicit in operation wrappers.
- URL hygiene: endpoint normalization centralizes path composition.
- Update source restrictions: self-update path validates expected host family before binary download.
- Self-update integrity verification: staged binaries are checksum-verified against release-manifest SHA256 before chmod/install.
- Timeout controls: review polling uses bounded timeout and cancellation semantics.
- Diagnostic hardening: parse-error helpers improve unsafe endpoint detection and operator triage.
- Setup secret-handling hardening: create-key failures avoid response-body echo and connector setup errors redact submitted provider keys.

## Known Gaps And Follow-Ups

| Gap | Why It Matters | Follow-Up |
| --- | --- | --- |
| Retry and backoff strategy is not centralized in network client wrappers | Transient failures can reduce reliability and affect user trust | Decide whether retries belong in network layer or explicit call sites, then document policy |
| 429/rate-limit handling is not represented as a standard control in operation docs | Can affect enterprise traffic reliability expectations | Document expected behavior and operator guidance for rate-limit scenarios |
| Proxy forwarding operation is intentionally flexible and may need tighter guardrails for some deployments | Security teams may ask for explicit path/method constraints | Document deployment-time constraints and threat model assumptions |
| Binary download integrity chain is split across components and not summarized here | Procurement and CISO reviews expect clear integrity story | Addressed: integrity path now documented with checksum verification owner at self-update staging control point |

## Review Cadence

- Update this file when any function is added/removed/renamed in the network package.
- Re-evaluate severity when auth model, payload sensitivity, or execution criticality changes.
- Security review trigger: any new High operation or changes to auth headers, proxy behavior, update download logic, or timeout/error policy.
