# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed — CLI v0.0.25

- `hera-agent exec` no longer hangs in non-TTY shells where stdin is open but
  will never deliver an EOF — Cursor's shell, bash `$(...)` capture, compound
  `cmd1; hera-agent exec ...`, and CI runners with detached stdin. Cursor users
  no longer need the `$null |` workaround before `exec` calls. Real pipes
  (`echo 'code' | hera-agent exec`) and file redirects (`hera-agent exec < code.cs`)
  keep working unchanged.

## [0.0.24] - 2026-05-20

### Changed — Connector 0.0.20 / CLI v0.0.24

Response token-cost reduction pass (see `docs/issues/token-cost-reduction.md`).
All items below shrink the bytes a calling agent reads back without changing
the underlying tool semantics.

**Non-breaking (CLI):**
- CLI emits compact JSON instead of 2-space-indented when stdout is piped
  (any non-TTY, or `HERA_AGENT_QUIET=1`). Human terminals still see indented.
- `[hera-agent] compiling...` stderr banner suppressed in non-TTY mode.
- `Update available:` notice suppressed in non-TTY mode.

**Connector — breaking defaults (override available):**
- `exec --depth` default lowered from `3` to `1`. Unity Objects
  (`Transform`, `GameObject`, `Scene`, ...) now serialize as
  `{name, type, instanceID}` unless `--depth >= 3`. Pass `--depth 3` to
  restore the prior deep-reflection behavior.
- `exec` runtime errors default to `--stacktrace user`: internal Unity /
  System / reflection frames are filtered out. Pass `--stacktrace full`
  for the previous raw stack, or `--stacktrace none` to omit it entirely.
- `exec` compile-error message is now `Compile error: L<n> CS<code>: <msg> (+N more)`.
  The full structured list is still in `data.compile_errors` — agents
  branching on `code == "EXEC_COMPILE_ERROR"` are unaffected; code that
  string-matched the multi-line message format will need updating.
- `console --lines` defaults to `20` (was unlimited). Use `--lines 0` for
  unlimited.
- `console` response omits `since`, `last_cursor`, `truncated`,
  `total_in_console`, `matched` fields when they equal their trivial
  default (no pagination, no filtering, no truncation).
- Collection truncation marker inside `exec` serialized return changed
  from `{"__truncated": true, "returned": 100, "hint": "..."}` to the
  short string `"__truncated:100"` (appended as the last item).



## [0.0.9] - 2025-05-15

### Fixed
- Windows `uninstall` command PowerShell script parsing error
  - Multi-line script caused `GetFullPath` exception and `CommandNotFoundException`
  - Compressed to single-line expression for `-Command` compatibility
- Windows `uninstall` self-deletion "Access is denied" error
  - Uses deferred deletion (`cmd /c timeout && del`) when direct removal fails
- Added `$legacy` empty-string guard and legacy directory existence check

## [0.0.8] - 2025-05-15

### Changed
- Author section enhanced with professional background and contact links
- Issue templates added for bug reports, feature requests, and questions
- README.ko.md Author section synchronized with English version
- docs/issues/powershell-exec-escaping.md updated to Lite standard
- docs/porting/ removed (pro-specific content)
- README Commands table updated with install/uninstall commands

## [0.0.7] - 2025-05-13

### Added
- Install flow: AI agent discovery prompt and rule anchoring after installation
- Porting guide for synchronizing changes with hera-agent-pro

### Changed
- README banner updated to `hera_lite.png`
- Log prefix unified from `[UnityCliConnector]` to `[Hera]`

## [0.0.6] - 2025-05-12

### Fixed
- Windows install location moved to `%LOCALAPPDATA%\Microsoft\WindowsApps` to eliminate IDE restart requirement

## [0.0.5] - 2025-05-11

### Fixed
- Uninstall: IDE restart guidance consistency and Common Issues updated

## [0.0.4] - 2025-05-10

### Fixed
- Windows PATH double-backslash bug
- IDE recognition missing guidance

### Added
- PROGRESS.json for tracking development milestones
- Change checklist expanded to include docs/ and CLAUDE.md

## [0.0.3] - 2025-05-09

### Changed
- Namespace and attributes renamed from `UnityCliConnector` to `HeraAgent`
- README install and QuickStart simplified
- Demo GIF placeholder added

## [0.0.2] - 2025-05-08

### Added
- Windows uninstall functions (`removeFromPATH`, `removeBinaryAndDir`)
- Install/uninstall commands

### Changed
- Rebrand from `unity-agent-cli` to `hera-agent`
- README elevated to brand manifesto tone
- Release binary names unified to `hera-agent-*`
- Connector displayName changed to `Hera Agent Lite`

## [0.0.1] - 2025-05-07

### Added
- Initial release: Control Unity Editor from terminal
- Core commands: `editor`, `exec`, `console`, `test`, `menu`, `screenshot`, `profiler`, `reserialize`, `list`, `status`
- Auto-start C# connector with `[HeraTool]` attribute-based tool registration
- Cross-platform support (Linux, macOS, Windows)
- HTTP bridge between Go CLI and Unity Editor
