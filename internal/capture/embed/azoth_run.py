"""Azoth Capture launcher shim.

Usage (done by the app, not by hand):
    python azoth_run.py <nw_capture dir> <patched _dtls_ledger.js> <patched _common.js> <nw_capture.py args...>

Why: Aeternum-World's _dtls_ledger.js hardcodes hook addresses for one game
build, and its _common.js finds the game in memory by the name NewWorld.exe.
The app resolves the right addresses from the game exe (internal/hooks), takes
the name from the game file, writes patched COPIES of both scripts, and this
shim points the upstream runner at the copies. The upstream checkout is never
modified.

Everything else (Frida spawn, ledger writing, HTTPS tap) is upstream's own
nw_capture.py, run unmodified.
"""
import runpy
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) < 4:
        print("usage: azoth_run.py <nw_capture dir> <patched _dtls_ledger.js> <patched _common.js> "
              "[nw_capture.py args...]")
        return 2
    nw_dir = Path(sys.argv[1]).resolve()
    patched = Path(sys.argv[2]).resolve()
    common = Path(sys.argv[3]).resolve()
    script = nw_dir / "nw_capture.py"
    for p in (script, nw_dir / "_runner.py", patched, common):
        if not p.is_file():
            print(f"-- azoth: missing file: {p}")
            return 2

    sys.path.insert(0, str(nw_dir))
    import _runner  # upstream module; reads COMMON_JS and DTLS_LEDGER_JS at spawn time

    # If upstream ever renames these, the override would silently do nothing and
    # the unpatched scripts would run. Refuse instead.
    for name in ("COMMON_JS", "DTLS_LEDGER_JS"):
        if not hasattr(_runner, name):
            print(f"-- azoth: upstream _runner.py has no {name}; refusing to run unpatched hooks")
            return 2
    _runner.COMMON_JS = common
    _runner.DTLS_LEDGER_JS = patched
    print(f"-- azoth: hook script = {patched}")
    print(f"-- azoth: helper script = {common}")

    sys.argv = [str(script)] + sys.argv[4:]
    runpy.run_path(str(script), run_name="__main__")
    return 0


if __name__ == "__main__":
    sys.exit(main())
