## Azoth Capture 0.2.0

**Fixes the capture for New World build 1.460.6033.6024067.**
The previous version hooked fixed addresses that this game patch moved, which
crashes the game or records garbage.

- Hook addresses are now found by machine-code signature, per game build, and
  the app refuses to start (without launching anything) if it cannot find all
  three exactly once. See `docs/HOOKS.md`.
- The Aeternum-World capture engine is pinned to a fixed commit instead of
  following upstream.
- The installer script is refreshed on every start, so existing installs pick up
  the pin.
- The Activity Log now shows the game build and the hook addresses it found.
- Docs: new README ("Is this a virus?"), `NOTICE.md`, `SECURITY.md`; the
  tutorial no longer promises you cannot be banned, and no longer mentions a Zip
  button the app does not have.

Check the download against `SHA256SUMS.txt`, or with
`gh attestation verify AzothCapture.exe --repo KyzaBOY/azoth-capture`.

Windows SmartScreen may warn: the exe is not code-signed. The source is in this
repository; see the README.
