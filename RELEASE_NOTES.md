## Azoth Capture 0.3.0: Linux support

The app now builds and runs on Linux as well as Windows, from one code base.
The output is the same: `ledger.bin` and `steps.json`. Contributed by
[@PikkonMG](https://github.com/PikkonMG) in #2.

Download `AzothCapture-linux-amd64` for Linux (setup in `docs/LINUX.md`) or
`AzothCapture.exe` for Windows. Check either one against `SHA256SUMS-linux.txt`
/ `SHA256SUMS.txt`, or with `gh attestation verify <file> --repo KyzaBOY/azoth-capture`.

**Windows:** nothing new to use. The capture code was split per platform and
Stop now waits for the ledger to finish writing, but this build has not had a
live Windows capture yet. If 0.3.0 gives you trouble on Windows, 0.2.0 keeps
working on game build 1.460.6033.6024067; please open an issue.

**Status on Linux:** a live 14-minute session under GE-Proton 10-12 (KDE Plasma,
Wayland) stayed on one connection and decoded with the upstream
`decode_dtls_ledger.py` without errors.

- On Linux you start the game the normal way through Steam, so Easy Anti-Cheat
  stays on. The capture attaches to the running game under Proton instead of
  launching it without the anti-cheat. In testing, a 14-minute session stayed
  on one connection the whole time, with none of the Windows kicks.
- The app attaches through a Windows Frida server running inside the game's
  Proton prefix. It never uses a native Linux attach, which terminates the game.
- It finds the game, the Proton version and the Steam Linux Runtime by reading
  Steam's own files, so nothing is hard coded. Use whichever Proton version runs
  the game on your system.
- Before it places any hook, it checks that the code at each hook site in the
  game's memory matches the executable file, and hooks nothing if it doesn't.
- It records only the DTLS ledger and leaves out the upstream HTTPS script,
  which swaps a game callback pointer that would be left dangling once the
  capture stops while the game keeps running.
- The Linux installer verifies the Frida server download by SHA-256 and keeps
  the Windows installer's pinned capture-engine commit and its refusal to run
  as root.
- The F8 key reads the keyboard from the kernel and needs the `input` group;
  `docs/LINUX.md` explains what that allows. The Mark button works without it.
- Optional video of the game window, off by default. Once it is turned on, a
  Start video button or F9 starts and stops clips while the capture runs, and a
  clip stops whenever the capture ends. It uses GPU Screen Recorder, records the
  window the player picks once in the desktop's share dialog (or a monitor they
  choose where that dialog doesn't exist), and records only the game's own
  sound, never a microphone. Each clip comes with a `video.json` holding its
  first-frame time on the ledger's clock. Video stays in the session folder and
  never goes into `send`.
- Setup and building are in `docs/LINUX.md`, and the tutorial has a new
  "Playing on Linux" section.

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

**Status on 1.460.6033.6024067:** works. A live 2-minute session was captured and
decoded with the upstream `decode_dtls_ledger.py`: 6,666 records in both
directions, no decode errors, no agent errors.

Check the download against the `SHA256SUMS.txt` release asset, or with
`gh attestation verify AzothCapture.exe --repo KyzaBOY/azoth-capture`.

Windows SmartScreen may warn: the exe is not code-signed. The source is in this
repository; see the README.
