# Upstream Audit

Snapshot: 2026-11-26. Upstream remains
[`yetone/magpie`](https://github.com/yetone/magpie). The fork `main` is an
exact mirror; `panel` contains maintenance documentation, the upstream-sync
workflow, and verified local integrations. Upstream `main` is at
`6346f383`.

## Upstream Sync

The panel now also includes the latest upstream changes:

- `6346f383`: a private-network Remote magpie address proves it holds the
  gateway key before a keyed request is sent to it;
- `1f5b883a`: a Remote magpie can keep alternate LAN, Tailscale or EasyTier
  addresses and ask the one that answers;
- `eb8402c3`: a remote magpie's dropped routing pick stays dropped after an
  editor saves the list;
- `5874a21a`: a routing group's classifier prompt is the user's own and does
  not name magpie as an assistant;
- `48fbe194`: a new session is not spread to a relay with no reads while a
  subscription with quota leads;
- `a7bd7a0f`: an exhausted OpenCode Go paid window no longer holds back its
  free models;
- `472b2c07`: the Library can show the skills used by one agent;
- `ad51ddfa`: DSH NEXT and DSH Desktop are recognized as DeepSeekRuntime's
  desktop app;
- `8bed5427`: a small Gemini reply limit now leaves room for thinking above
  it;
- `3713903b`: an import app uninstalled from the Mac is no longer listed as
  added;
- `03e53d19`: Nowledge Mem pointed at another machine belongs to the user,
  and Disconnect leaves it alone;
- `1bbd0aae`: a DSH routing group goes on the route every member is best on;
- `ef7acbcc`: near its cap, an account stops a turn short by the amount that
  turn spent there;
- `8a1498a4`: a shared gateway keeps the computer awake while it is shared;
- `108b72ee`: an upgraded account row reports its current plan and draws its
  Fable window;
- `82127a85`: Zhipu's Chinese 1308/1310 responses rest the account as quota
  until the reported window reset.

It also retains upstream `eab4b006` and the five commits before it:

- `bd0ac4ac`: CatPaw IDE conversation discovery;
- `b94ffbec`: avoid counting encrypted GPT reasoning twice;
- `af75743b`: include disabled accounts and keys in account tests;
- `f7cb8aa0`: return complete account model lists from the gateway;
- `eab4b006`: fix Codex takeover when its own model and a Magpie model are
  wired side by side.

The Reasonix integration from `#1033` and CatPaw discovery are both retained in
the shared session listing and transcript switches.

## Integrated Open Issues

| Issue | Current state | Verification |
| :--- | :--- | :--- |
| [#7758](https://github.com/yetone/magpie/issues/7758) | Integrated in panel as `435916f`; [clean upstream compare](https://github.com/yetone/magpie/compare/main...L0NE-6:fix/7758-agy-image-url-upstream) | Before building an agy-cli Messages request, URL-only images are fetched through the existing public-address guard and inlined as base64 on a copied request. This covers Gemini `fileData.fileUri`, OpenAI `image_url`, Anthropic URL sources and images attached to tool results without changing the original request used by another provider or retry. Loopback URLs remain blocked and non-image responses fail. Focused tests, 5 race iterations and vet pass; no live agy-cli account was exercised. |
| [#7725](https://github.com/yetone/magpie/issues/7725) | Integrated in panel as `e9c41b7`; [clean upstream compare](https://github.com/yetone/magpie/compare/main...L0NE-6:fix/7725-workbuddy-cos-url-upstream) | WorkBuddy image responses now map only the known `bucket.cos-internal.<region>.tencentcos.cn` VPC host to `bucket.cos.<region>.myqcloud.com`, preserving scheme, port, path and query signature. The drawing regression checks that clients receive the public URL and no internal host. Focused tests, 5 race iterations and vet pass; no live WorkBuddy image request was made. |
| [#7761](https://github.com/yetone/magpie/issues/7761) | Integrated in panel as `1f1a02c`; [clean upstream compare](https://github.com/yetone/magpie/compare/main...L0NE-6:fix/7761-group-protocol-upstream) | Routing groups for Pi, omp, droid, Hermes, Grok Build, LiveAgent and DSH now use the APIs every member shares instead of falling back to Chat. Messages-only groups stay on Messages, Gemini-capable agents use Gemini where available, and unsupported agents keep the shared Messages route. Pi and omp also carry the group's all-adaptive Messages choice into their thinking controls. Focused group tests, 3 race iterations and vet pass; no live vendor request was made. |
| [#7512](https://github.com/yetone/magpie/issues/7512) | Integrated in panel as `2bf6d2b`; [clean upstream compare](https://github.com/yetone/magpie/compare/main...L0NE-6:fix/7512-subagents-measure-upstream) | The Library's Subagents row is now selected, checked for connection and stylesheet state, and measured in one page turn, retrying until the computed line-height is finite. This removes the detached-row `NaN line` race at both narrow and wide widths. Chromium and WebKit each pass all six cases across five languages; the full GUI suite was not run locally. |
| [#7762](https://github.com/yetone/magpie/issues/7762) | Integrated in panel as `134c4fb`; [clean upstream compare](https://github.com/yetone/magpie/compare/main...L0NE-6:fix/7762-library-measure-upstream) | The Library's command and hook rows are now selected, checked for connection and stylesheet state, and measured in one page turn, retrying until the computed line-height is finite. This removes the detached-row `NaN line` race and also covers the two siblings called out by `#7512`. Chromium passes all 12 cases. WebKit passes all Hooks cases, while the current upstream `library-commands` file still has a separate greyed-chip failure; the full GUI suite was not run locally. |
| [#5192](https://github.com/yetone/magpie/issues/5192) | Integrated in panel as `b7a0eb2`; [clean upstream compare](https://github.com/yetone/magpie/compare/main...L0NE-6:fix/5192-hook-session-doc-upstream) | Hook documentation and the `GatewayContext.Session` field now state that `session` is the client's declared id, is empty when no session header is sent, and is never inferred from body fields. A focused gateway test pins precedence, trimming and the 128-byte cap. The focused test and `go vet` pass; plugin-host runtime behavior is unchanged. |
| [#7733](https://github.com/yetone/magpie/issues/7733) | Integrated in panel as `88c7e73b` | A named `tool_choice` now keeps the client's complete tool list for the run while `allowedTools` still permits only the named tool for that turn. The focused gateway regression tests and `go vet ./internal/gateway` pass. |
| [#7746](https://github.com/yetone/magpie/issues/7746) | Integrated in panel as `8ef1bd0e` | A gateway call with a matching `response_id` now pairs with its Codex session-file row even when another client, such as T3 Code, drove Codex. The focused usage regression test and `go vet -tags nogui ./internal/usage ./internal/gateway` pass. The full `internal/usage` package still has an unrelated Windows account-metadata failure. |
| [#7738](https://github.com/yetone/magpie/issues/7738) | Fixed in upstream `eab4b006` | For GPT-6.2 and later, Anthropic-to-Responses conversion places the system prompt first in `input` as a developer message instead of in top-level `instructions`. `TestResponsesSystemFirstInInputOnGPT62`, `TestInstructionsAfterInput` and `TestCodexBodyInstructionsFirstInInputOnGPT62` pass. |
| [#7723](https://github.com/yetone/magpie/issues/7723) | Integrated in panel as `23542477`, with follow-up `58545de7` | Shape stand-ins are restored, held across SSE chunks and excluded from other masks only as whole names. The follow-up keeps case-folded username stand-ins visible at stream boundaries. The full `internal/redact` suite and `go vet -tags nogui ./internal/redact` pass. The separate process-global stand-in table isolation concern from the issue discussion remains open. |
| [#7744](https://github.com/yetone/magpie/issues/7744) | Integrated in panel as `0002112c` | `mcpProxy` flushes `text/event-stream` response headers immediately after `WriteHeader`, so a silent standalone GET no longer blocks MCP initialization. The direct and restricted-key regression tests pass, and the focused race run passes 20 iterations. |
| [#7751](https://github.com/yetone/magpie/issues/7751) | Integrated in panel as `ae3f8d2f` | MCP discovery now checks RFC 9728 protected-resource metadata before treating a `2xx initialize` response as an open server. Google Workspace-style lazy-auth servers can sign in, while a 200 response without metadata still returns `ErrNoSignIn`. Focused tests, race and vet pass. |
| [#7729](https://github.com/yetone/magpie/issues/7729) | Integrated in panel as `acd8dccf` | `serveWSL` leaves listeners to `http.Server.Shutdown` instead of closing them as its context ends, avoiding a false truncated-request error on Windows. Windows `WSL` tests, 10 race iterations and vet pass. |
| [#7750](https://github.com/yetone/magpie/issues/7750) | Integrated in panel as `4b8b4d1a` | Obsidian config and vault probes are bounded to one second; a stalled path is treated as no Copilot and remembered once per process. Obsidian tests, the focused race test and vet pass. A real macOS File Provider stall remains unverified. |
| [#7747](https://github.com/yetone/magpie/issues/7747) | Integrated in panel as `cb29c57f` | Each Claude run gets a unique ID and an optional `MAGPIE_CLAUDE_KILL` hook runs after the host tree ends, covering Docker/SSH/WSL exec boundaries. Abort waits for both cleanup paths. Focused gateway/TUI tests, race, native/Linux vet and Linux test compilation pass; no real Docker or WSL runtime was available. |
| [#7749](https://github.com/yetone/magpie/issues/7749) | Integrated in panel as `a117be61` | A bare `~/.claude-* /settings.json` no longer makes a Claude Code profile unless it contains a key Claude Code or magpie writes. claude-mem is no longer listed or ticked, while credentials, projects and magpie's isolated Desktop Code tab still count. Focused tests and race pass. |
| [#7707](https://github.com/yetone/magpie/issues/7707) | Integrated in panel as `20539962` | Plain-text Anthropic cyber/content policy refusals from a 400, 502 or streamed `api_error` are classified as refusals, fail over to another provider/model and do not rest the account. Unrelated 400/502 messages keep their old behavior. Refusal-family tests, race and vet pass. |
| [#7742](https://github.com/yetone/magpie/issues/7742) | Integrated in panel as `16871d9c` | A timed-out `magpie quota` now reports subscription cards that already landed, falls back to the last kept card for the rest, and labels a never-read signed-in subscription as "not read in time" instead of a fabricated balance configuration error. CLI/provider tests, race and vet pass. |
| [#7755](https://github.com/yetone/magpie/issues/7755) | Integrated in panel as `88236698` | The artifact list/delete test now waits until the millisecond clock has moved past the previous artifact's timestamp before republishing it. The deterministic ID tie-break is retained, and the test passed 100 consecutive runs. The full artifact package still has an unrelated Windows file-mode assertion failure. |
| [#7702](https://github.com/yetone/magpie/issues/7702) | Integrated in panel as `576290ab` | A relayed Gemini request ending on a model turn gets the same synthetic user continuation as the translated non-streaming path. Function-response tails remain untouched. The streaming regression test, race and vet pass. |
| [#7753](https://github.com/yetone/magpie/issues/7753) | Integrated in panel as `f0f52aa6` | Custom-tool argument streaming no longer aborts on JSON-invalid escapes, bare control characters or unescaped quotes; standard escapes, surrogate pairs, split UTF-8 and truncated escapes are still normalized. Streamed deltas and finished inputs share one normalizer. 31 focused tests, race and vet pass. |
| [#7745](https://github.com/yetone/magpie/issues/7745) | Integrated in panel as `efb02eb` | Cursor and WorkBuddy plugin hosts now replay an earlier turn's `reasoning_content`; neither is sent a `noReasoning` placeholder. Focused, 10-repeat race and vet checks pass with synthetic plugin fixtures. Real Cursor and WorkBuddy accounts were unavailable, so live A/B behavior remains unverified. |
| [#7728](https://github.com/yetone/magpie/issues/7728) | Integrated in panel as `182aae5` | Relayed Responses, Messages and Gemini replies now drop calls that name no function, including streamed fragments and final output, and renumber retained items. Nameless Gemini request parts are removed too, while the Chat relay handles an empty first name. Nameless/ChatTidy tests, 5 race iterations and vet pass; real vendor streams were not available. |
| [#7668](https://github.com/yetone/magpie/issues/7668) | Integrated in panel as `5fdd9bd` | Equal text replayed in both `reasoning` and `reasoning_content` is folded to one `reasoning_content` for Raccoon's SenseNova Flash and SenseNova Token Plan models; distinct values, non-string values, `reasoning_details`, and tool-message fields stay untouched. Focused tests, 5 race iterations and vet pass; live vendor accounts were not available. |
| [#7752](https://github.com/yetone/magpie/issues/7752) | Integrated in panel as `7702e4a7` | GUI tests now follow the existing responsive layout: the standalone Artifacts tab is expected at 1024px and above, while narrower layouts use the Library header's Artifacts entry. Chromium and WebKit each pass 2/2. |
| [#7757](https://github.com/yetone/magpie/issues/7757) | Integrated in panel as `27e8ba0` | Publish timestamps now advance strictly in publish order, including two publishes in one millisecond and a wall clock that moves backwards. A restarted process resumes from the newest stored artifact timestamp. The deterministic same-millisecond regression and CLI test pass 100 consecutive runs, 10 race iterations and vet. |
| [#7754](https://github.com/yetone/magpie/issues/7754) | Integrated in panel as `9c4faf6` | Listener ownership now matches the gateway's local address on Darwin, Linux `/proc` and Windows, while the port-only API keeps its compatibility semantics. A real same-port `127.0.0.1`/`127.0.0.2` regression passes; focused proc, gateway and GUI tests, vet and Darwin/Linux cross-compilation pass. |
| [#7739](https://github.com/yetone/magpie/issues/7739) | Integrated in panel as `81739e2` | `usage.jsonl` now retires rows older than the 30-day retention window once the ledger reaches its size floor, keeps concurrent appends, invalidates the derived index, and recovers an interrupted Windows replacement. Focused tests, race and vet pass. |
| [#7735](https://github.com/yetone/magpie/issues/7735) | Integrated in panel as `41a675d` | The memory-row GUI test now waits for fonts and the folded/open height state before measuring; the target Chromium file passes 11/11 alone, and the related memory batch passed its target subtests under concurrent load. A separate pre-existing gateway i18n key failure remains outside this fix. |
| [#7748](https://github.com/yetone/magpie/issues/7748) | Integrated in panel as `42d7ebd` | agy account routing now matches a model list by its model family, so a model owned only by a local Gemini account stays on that account instead of being offered to every pool. Family and raw-effort routing tests, race and vet pass. |
| [#6699](https://github.com/yetone/magpie/issues/6699) | Integrated in panel as `a2a40af8` | When `~/.agents/skills` aliases the library's own skills directory, shared-entry discovery, ownership, hand-over and hand-back all stop treating the library's entries as external user entries. Tick, symlink and Windows-junction tests pass. |
| [#6697](https://github.com/yetone/magpie/issues/6697) | Integrated in panel as `e20be33` | Sync rebuilds a missing folder skill entry from `source.dir`, carries the dangling state across per-step reloads, removes Magpie's broken agent links, and reports skills whose source is also gone. Focused, related, race and vet checks pass. |
| [#7740](https://github.com/yetone/magpie/issues/7740) | Integrated in panel as `e4ac00b` | The directory hash memo now persists across `magpie library sync` processes and is validated by path, type, size and mtime. The regression drops a second-process read from 6,329,271 bytes to 38,365 bytes. Focused, related, race, vet and Linux test compilation pass. |

## High-Risk Open Issues

| Issue | Current state | Evidence boundary |
| :--- | :--- | :--- |
| [#7438](https://github.com/yetone/magpie/issues/7438) | Open; no deterministic reproduction or patch | Static audits found no deletion path in the update apply flow or the resident plugin JavaScript. The update swap is two file renames and the idle transition performs no filesystem cleanup. The remaining suspects are the external `agy` 1.5.14 artifact, whose fingerprint is no longer retrievable, and a machine-local idle-triggered action outside the captured USN window. Sysmon is now installed on the reporter's machine. Do not mark this fixed without a process-level reproduction. |
| [#7708](https://github.com/yetone/magpie/issues/7708) | Open product decision; no patch under the current contract | Upstream's check shows the search provider receives only magpie's isolated search ask, not the agent conversation, and the Routing row records `kind: web_search` with the originating agent/model. Native search is a global Automatic slot; `visible` and `hiddenModels` narrow model lists but do not scope that slot. Per-agent search isolation needs a maintainer decision on fallback semantics rather than a mechanical fix. |
| [#7743](https://github.com/yetone/magpie/issues/7743) | Open security/design boundary | Remote artifact pages must not reach the local store, Claude, self-publish or assets without a capability-token and Origin contract that covers `null`, `*.localhost`, missing Origin and rebound Host. A mechanical route enablement would widen a subscription-spending write surface; keep it open until the host and token model is specified. |
| [#7756](https://github.com/yetone/magpie/issues/7756) | Open product decision | Project-level plugin resource writers need a stable official contract before a panel implementation can define ownership, precedence and migration behavior. |

## Product and Feature Decisions

| Item | Current state | Boundary |
| :--- | :--- | :--- |
| [#7743](https://github.com/yetone/magpie/issues/7743) | Open design | Serving an artifact's store, Claude bridge, self-version writes and blobs from another computer needs a capability-token scope, same-site write protection and a fresh Origin/null/Host-rebinding review for every route. A mechanical patch would widen a remote trust boundary without an upstream contract. |
| [#7741](https://github.com/yetone/magpie/issues/7741) | Open feature | Bulk import of Codex enterprise workspace accounts needs a vendor API and UX contract for credentials, account deduplication, failures and revocation. It is not a regression that can be fixed safely from the current provider interface alone. |
| [#7756](https://github.com/yetone/magpie/issues/7756) | Open design | A declarative project-scoped resource-writer contract must preserve the Library's single-writer ledger, path confinement, `MainOnly` worktree rules, clean withdrawal and plugin reload isolation. Implementing only a filesystem hook would bypass those invariants. |

## Open Pull Requests

| PR | Current state | Maintenance note |
| :--- | :--- | :--- |
| [#7760](https://github.com/yetone/magpie/pull/7760) | Integrated in panel as `a1450b7` | A hook's Responses reply now keeps the streamed `output_item.done` items when `response.completed` names an empty output, accepts `response.incomplete` as the reply for a cut stream, and recognizes an event type carried only on the `event:` line. Focused hook tests, 5 race iterations and vet pass; a real vendor hook was not exercised. |
| [#136](https://github.com/yetone/magpie/pull/136) | Integrated in panel | Ported the isolated identity-forget behavior as `c6f5056`; focused tests and race checks pass. |
| [#777](https://github.com/yetone/magpie/pull/777) | Conflicting / dirty | P3. Needs a current multi-login fixture; may break one-click sign-in. |
| [#930](https://github.com/yetone/magpie/pull/930) | Integrated in panel | Windows npm hardlink update path is `ab72db8`; focused tests, repeated runs, vet and gofmt pass. Real npm/UAC update remains unverified. |
| [#951](https://github.com/yetone/magpie/pull/951) | Conflicting / dirty | P3 wording change; product naming decision still required. |
| [#977](https://github.com/yetone/magpie/pull/977) | Conflicting / dirty | P2 usage forecast; needs rebase against the current Usage layout. |
| [#978](https://github.com/yetone/magpie/pull/978) | Conflicting / dirty | P2 Usage layout; needs narrow-screen and Playwright regression checks. |
| [#1017](https://github.com/yetone/magpie/pull/1017) | Integrated in panel | Reimplemented for the current Usage architecture as `8a6b1c1`; Chromium regression tests pass, WebKit remains unverified. |
| [#1020](https://github.com/yetone/magpie/pull/1020) | Integrated in panel | CI-only installed-font test integrated as `f81f1a4`; upstream remains open. |
| [#1033](https://github.com/yetone/magpie/pull/1033) | Integrated in panel | Reasonix legacy transcripts, native ledgers and all four store codecs are integrated as `1884c552`, with compatibility and retained-usage UI. Focused sessions and GUI tests pass; the Python preview tools pass 13 tests. No live Reasonix installation was available. |
| [#1072](https://github.com/yetone/magpie/pull/1072) | Conflicting / dirty | P2 library hash behavior; preserve old-hash compatibility. |
| [#1073](https://github.com/yetone/magpie/pull/1073) | Integrated in panel | Reimplemented against the current Settings API as `b2ed15e`; focused Go tests and race checks pass. |
| [#1074](https://github.com/yetone/magpie/pull/1074) | Integrated in panel | Archive and OTLP masking are unified as `4ecf1ef`; focused Go tests and cross-platform vet pass. The later panel merge dropped `Settings.Redaction`/`Scrubbing`, restored as `d1469ff0`. Real S3/collector verification remains pending. |
| [#1157](https://github.com/yetone/magpie/pull/1157) | Conflicting / dirty | P2 CI decision: run the whole provider race suite. |
| [#1181](https://github.com/yetone/magpie/pull/1181) | Conflicting / dirty | P2 routing key plan windows; adapt to the current `cardFor` refactor. |
| [#1258](https://github.com/yetone/magpie/pull/1258) | Integrated in panel | SIGTERM and SIGINT now drain and return through `Web.Wait`; SIGHUP remains signal-terminated by design. Linux subprocess test passed in Actions run `45608299051` for SIGTERM, SIGINT, SIGHUP and page Quit. Integrated as `a8377782`. |
| [#1331](https://github.com/yetone/magpie/pull/1331) | Integrated in panel | Loopback HTTPS, constrained certificate SANs, explicit GUI trust flow and allowlisted CORS are integrated as `532f91a`; real macOS/Windows Office trust chains remain unverified. |

## Maintenance Rules

- A PR being mergeable is not proof that its behavior is safe.
- A local panel integration is not an upstream merge.
- Do not claim macOS Office, Windows, Linux signal, or real-agent behavior unless that environment was actually exercised.
- The fork never publishes upstream releases or distribution channels.
