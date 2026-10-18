## Azoth Capture 0.2.0

**Adapts the capture to New World build 1.460.6033.6024067.**
The previous version hooked fixed addresses that this game patch moved, which
crashes the game or records garbage.

- Hook addresses are now found by machine-code signature, per game build, and
  the app refuses to start (without launching anything) if it cannot find all
  three exactly once. See `docs/HOOKS.md`.
- On a fresh install (or after Retry in Settings) the Aeternum-World capture
  engine is pinned to a fixed commit instead of following upstream. Existing
  installs keep the checkout they already have.
- The Activity Log now shows the game build and the hook addresses it found.
- Docs: new README ("Is this a virus?"), `NOTICE.md`, `SECURITY.md`. The
  tutorial no longer promises you cannot be banned, no longer says `ledger.bin`
  and `meta.json` are safe to share as they were (see "What NOT to share"), and
  no longer mentions a Zip button the app does not have.

**Status on 1.460.6033.6024067:** hooks are located and install correctly on a
suspended game process. A full live capture on this build is being confirmed.

Check the download against the `SHA256SUMS.txt` release asset, or with
`gh attestation verify AzothCapture.exe --repo KyzaBOY/azoth-capture`.

Windows SmartScreen may warn: the exe is not code-signed. The source is in this
repository; see the README.
