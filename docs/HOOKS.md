# How the hooks are placed

The capture needs three functions inside `NewWorld.exe`. The upstream scripts
call them `SSL_read`, `SSL_write` and `nss_keylog_int`:

| Function | Hooked to get |
|---|---|
| `SSL_read`  | the plaintext the server sent (after DTLS decryption) |
| `SSL_write` | the plaintext the client sent |
| `nss_keylog_int` | the TLS key log lines |

## Why not fixed addresses

Upstream's `_dtls_ledger.js` hardcodes the addresses for one game build. Every
game patch moves them. In build **1.460.6033.6024067** all three shifted by
`+0x1640`, and the old addresses landed in the middle of other functions.
Hooking mid-instruction crashes the game or records garbage, with no error
message to explain it.

## What the app does instead

1. `internal/hooks` opens the game exe and scans its `.text` for a **signature**
   of each function: its first 65-68 bytes, with every rip-relative
   displacement and relative call/branch target turned into a wildcard (those
   are the bytes a patch changes when it merely shifts code).
2. Each signature must match **exactly once**. Zero or several matches means the
   function changed or is ambiguous, and the app stops with a clear message
   *before launching anything*.
3. It writes a copy of `_dtls_ledger.js` with the address table replaced
   (`internal/capture/agent.go`) and starts the capture pointed at that copy.
   On Windows, `azoth_run.py` starts the upstream runner, which spawns the
   game. On Linux, `azoth_attach.py` attaches to the game that is already
   running and loads the same patched copy. Since that game is already
   running, it first compares the first 16 bytes of each hook site in memory
   with the bytes in the file (`hooks.ReadPrologues`) and places no hook if any
   of them differ. The upstream checkout is never modified on either platform,
   and the signature scan above is shared code (`internal/hooks`, pure
   `debug/pe`), so both place the hooks the same way.

`SSL_read` and `SSL_write` start with the same 18 bytes; they are told apart by
the error-code constants a few instructions later (a unit test guards this).

## How the signatures were made, and checked

They were cut from the previous build, where the hooks were known to work (the
bytes logged by real captures on 2026-10-13 match), then:

- compared with the same functions in 1.460.6033: the first 512 bytes of each
  are identical except for relocated 2-byte displacement fields, i.e. the code
  did not change, it only moved;
- confirmed to match the old bytes and to be **unique** in the new `.text`;
- confirmed at run time: with the patched script, Frida attached to a
  *suspended* 1.460.6033 process installs all three hooks and the first 16
  bytes at each target equal what the old, working captures logged.

`tools/gen_sigs.py` reproduces the signatures in `internal/hooks/hooks.go`
byte-for-byte from the exe:

```
pip install capstone
python tools/gen_sigs.py --exe NewWorld.exe --read 0x1478F2E20 --write 0x1478F3460 --keylog 0x1478F39E0
```

## When a patch changes these functions

The app will report "this game build is not supported" and nothing is launched.
To fix it:

1. Find the three functions in the new exe (a disassembler; `SSL_read` and
   `SSL_write` are adjacent and take `(ssl, buf, num)`; `nss_keylog_int` is
   called with a label, the client random and the secret). Upstream took its
   addresses from an IDA database (see the header of `_common.js`) and does not
   document how it found them, so expect to do this part by hand.
2. Run `tools/gen_sigs.py` with their addresses and paste the three strings into
   `internal/hooks/hooks.go`.
3. `AZOTH_NW_EXE=<path to NewWorld.exe> go test ./internal/hooks/` and add the
   build's SHA-256 and expected addresses to `real_windows_test.go`.
4. Do a live capture and decode it with `decode_dtls_ledger.py` before
   releasing.
