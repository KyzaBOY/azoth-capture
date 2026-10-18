# Azoth Capture

Records what the *New World: Aeternum* server sends to your game client, plus a
timestamped diary of what you were doing, so the community can rebuild the game
server before Amazon shuts it down on **31 January 2027**. It is the capture
tool of the [Azoth](https://projectazoth.com) preservation project.

This repository is the complete source of the app. It exists so you can read
what you are about to run.

> How to *use* it: [TUTORIAL.md](TUTORIAL.md).
> Not affiliated with Amazon. No Amazon code or assets are in here.

## Is this a virus?

Fair question: it downloads things, it launches your game without its
anti-cheat, and it injects into the game process. Those are exactly the
behaviours scanners and cautious people flag, so here is the whole picture
instead of "trust me". Everything below comes from this source; check it.

**What it does on your PC**

- Runs a small desktop window (Go + [Wails](https://wails.io)). No admin rights:
  the installer script *refuses* to run elevated.
- On first start it sets up a private Python + Git **inside the folder you put
  the exe in** (see the table below). Delete that folder and those tools are
  gone. Your settings live in `%LOCALAPPDATA%\AzothCapture` and exported
  sessions in `Documents\Azoth Captures`.
- When you press **Start**, it launches `NewWorld.exe` itself, **without Easy
  Anti-Cheat**, under [Frida](https://frida.re), and records the game's network
  traffic after decryption into a folder on your disk. That is what a capture
  *is*, and it is the part that makes antivirus nervous. Because the game's
  HTTPS traffic carries your session tokens, that local folder is sensitive;
  see "What to share" below.
- F8 (configurable) is a global hotkey registered with Windows'
  `RegisterHotKey`, to drop a marker in your diary. It is not a keylogger: it
  receives only the one key you chose.
- On **Stop** it writes a `send` folder with the two files we ask for.

**What it sends out:** one request, `GET https://projectazoth.com/api/needed`,
to show which captures the community still needs. No body, no identifiers, no
telemetry. The upstream scripts it runs (`nw_capture.py`, `_runner.py`,
`extract_https_pairs.py` and the Frida agents) import no networking code; read
them at the pinned commit. Your files only leave your PC if *you* post them.

**What it does not do:** write to the registry, install a service or startup
entry, or upload anything. The code has no such calls; search for them.

**What it downloads** (all into the app's own folder, all from the official hosts):

| What | From | Integrity |
|---|---|---|
| Python 3.12.9 embeddable | `python.org/ftp/python/3.12.9/` | HTTPS, version-pinned |
| pip bootstrap | `bootstrap.pypa.io/get-pip.py` | HTTPS only (the file is not versioned) |
| Portable Git 2.45.1 | `github.com/git-for-windows/git/releases` | HTTPS, version-pinned |
| Capture engine: [Aeternum-World](https://github.com/Coldzer0/Aeternum-World) | `github.com/Coldzer0/Aeternum-World` | **pinned to one fixed commit** (`820156d`), so later upstream changes never reach you |
| Python packages (Frida etc.) | PyPI | exact versions (`==`) from that repo's `requirements.txt` |

The whole download list is in
[`internal/capture/embed/install_env.bat`](internal/capture/embed/install_env.bat),
about 360 lines of plain batch.

**Honest limits.** The Python, pip and Git downloads are version-pinned over
HTTPS but not hash-checked yet. The exe is not code-signed, so Windows
SmartScreen will warn about it and some scanners will give it a heuristic hit.
Running the game without EAC is against its rules; read the ban note in the
[tutorial](TUTORIAL.md#the-eac-situation-read-this) and decide for yourself.

## Check it yourself

**Build it** (Windows, [Go](https://go.dev) 1.26+):

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
wails build -clean -platform windows/amd64 -trimpath
# result: build\bin\AzothCapture.exe
```

**Or verify a release.** Releases are built by GitHub Actions
([`.github/workflows/release.yml`](.github/workflows/release.yml)) from the
tagged commit, with a `SHA256SUMS.txt` and a signed build attestation:

```powershell
(Get-FileHash .\AzothCapture.exe -Algorithm SHA256).Hash   # compare with SHA256SUMS.txt
gh attestation verify .\AzothCapture.exe --repo KyzaBOY/azoth-capture
```

**Where to look**

| Path | What it is |
|---|---|
| `internal/capture/` | Starts the installer and the capture; patches the hook script |
| `internal/capture/embed/install_env.bat` | The installer: everything that gets downloaded |
| `internal/capture/embed/azoth_run.py` | 20-line launcher that points the upstream runner at the patched script |
| `internal/hooks/` | Finds the three game functions we hook, by machine-code signature |
| `internal/journal/` | The diary (`steps.json`) and its export |
| `internal/hotkey/` | The F8 hotkey |
| `internal/needed/` | The one network request |
| `frontend/` | The window (plain HTML/CSS/JS, no framework, no external requests) |

## What to share, and what not

Share only the two files in the `send` folder: `ledger.bin` and `steps.json`.
`ledger.bin` is the plaintext of your session with the game server (it shows
what your character did), which is exactly what we need. **Never** share the
capture folder's `https/`, `logs/`, `keylog.txt` or `summary.json`: they hold
your account tokens and TLS keys.

## Supported game builds

The capture hooks three functions inside `NewWorld.exe`. A game patch moves
them, and a hook in the wrong place crashes the game, so the app **finds them
by signature and refuses to start if it cannot find all three exactly once**
([`docs/HOOKS.md`](docs/HOOKS.md)).

| Game build | Result |
|---|---|
| 1.460.6033.6024067 | hooks located and installed; see the release notes for the live capture result |

## Credits and licence

MIT, see [LICENSE](LICENSE). The capture engine is
[Aeternum-World](https://github.com/Coldzer0/Aeternum-World) by Coldzer0
(AGPL-3.0), used unmodified; details in [NOTICE.md](NOTICE.md). Report
problems as described in [SECURITY.md](SECURITY.md).
