# Upstream Audit

Snapshot: 2026-11-25. Upstream remains
[`yetone/magpie`](https://github.com/yetone/magpie). The fork `main` is an
exact mirror; `panel` contains maintenance documentation, the upstream-sync
workflow, and verified local integrations.

## Open Pull Requests

| PR | Current state | Maintenance note |
| :--- | :--- | :--- |
| [#136](https://github.com/yetone/magpie/pull/136) | Integrated in panel | Ported the isolated identity-forget behavior as `c6f5056`; focused tests and race checks pass. |
| [#777](https://github.com/yetone/magpie/pull/777) | Conflicting / dirty | P3. Needs a current multi-login fixture; may break one-click sign-in. |
| [#930](https://github.com/yetone/magpie/pull/930) | Integrated in panel | Windows npm hardlink update path is `ab72db8`; focused tests, repeated runs, vet and gofmt pass. Real npm/UAC update remains unverified. |
| [#951](https://github.com/yetone/magpie/pull/951) | Conflicting / dirty | P3 wording change; product naming decision still required. |
| [#977](https://github.com/yetone/magpie/pull/977) | Conflicting / dirty | P2 usage forecast; needs rebase against the current Usage layout. |
| [#978](https://github.com/yetone/magpie/pull/978) | Conflicting / dirty | P2 Usage layout; needs narrow-screen and Playwright regression checks. |
| [#1017](https://github.com/yetone/magpie/pull/1017) | Integrated in panel | Reimplemented for the current Usage architecture as `8a6b1c1`; Chromium regression tests pass, WebKit remains unverified. |
| [#1020](https://github.com/yetone/magpie/pull/1020) | Integrated in panel | CI-only installed-font test integrated as `f81f1a4`; upstream remains open. |
| [#1033](https://github.com/yetone/magpie/pull/1033) | Draft / conflicting | P2 Reasonix sessions; needs rebase, completeness tests, and a real binary check. |
| [#1072](https://github.com/yetone/magpie/pull/1072) | Conflicting / dirty | P2 library hash behavior; preserve old-hash compatibility. |
| [#1073](https://github.com/yetone/magpie/pull/1073) | Integrated in panel | Reimplemented against the current Settings API as `b2ed15e`; focused Go tests and race checks pass. |
| [#1074](https://github.com/yetone/magpie/pull/1074) | Integrated in panel | Archive and OTLP masking are unified as `4ecf1ef`; focused Go tests and cross-platform vet pass. Real S3/collector verification remains pending. |
| [#1157](https://github.com/yetone/magpie/pull/1157) | Conflicting / dirty | P2 CI decision: run the whole provider race suite. |
| [#1181](https://github.com/yetone/magpie/pull/1181) | Conflicting / dirty | P2 routing key plan windows; adapt to the current `cardFor` refactor. |
| [#1258](https://github.com/yetone/magpie/pull/1258) | Conflicting / dirty | P1 signal handling; needs Linux drain and ignored-SIGINT tests. |
| [#1331](https://github.com/yetone/magpie/pull/1331) | Conflicting / dirty | P1 local HTTPS for Microsoft 365; requires certificate lifecycle and trust-store review. |

## Maintenance Rules

- A PR being mergeable is not proof that its behavior is safe.
- A local panel integration is not an upstream merge.
- Do not claim macOS Office, Windows, Linux signal, or real-agent behavior unless that environment was actually exercised.
- The fork never publishes upstream releases or distribution channels.