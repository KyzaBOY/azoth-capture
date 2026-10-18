"""Azoth Capture launcher shim.

Usage (done by the app, not by hand):
    python azoth_run.py <nw_capture dir> <patched _dtls_ledger.js> <nw_capture.py args...>

Why: Aeternum-World's _dtls_ledger.js hardcodes hook addresses for one game
build. The app resolves the right addresses from the game exe (internal/hooks),
writes a patched COPY of that script, and this shim points the upstream runner
at the copy. The upstream checkout is never modified.

Everything else (Frida spawn, ledger writing, HTTPS tap) is upstream's own
nw_capture.py, run unmodified.
"""
import runpy
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) < 3:
        print("usage: azoth_run.py <nw_capture dir> <patched _dtls_ledger.js> [nw_capture.py args...]")
        return 2
    nw_dir = Path(sys.argv[1]).resolve()
    patched = Path(sys.argv[2]).resolve()
    script = nw_dir / "nw_capture.py"
    for p in (script, nw_dir / "_runner.py", patched):
        if not p.is_file():
            print(f"-- azoth: missing file: {p}")
            return 2

    sys.path.insert(0, str(nw_dir))
    import _runner  # upstream module; reads DTLS_LEDGER_JS at spawn time

    # If upstream ever renames this, the override would silently do nothing and
    # the old hardcoded addresses would run. Refuse instead.
    if not hasattr(_runner, "DTLS_LEDGER_JS"):
        print("-- azoth: upstream _runner.py has no DTLS_LEDGER_JS; refusing to run unpatched hooks")
        return 2
    _runner.DTLS_LEDGER_JS = patched
    print(f"-- azoth: hook script = {patched}")

    sys.argv = [str(script)] + sys.argv[3:]
    runpy.run_path(str(script), run_name="__main__")
    return 0


if __name__ == "__main__":
    sys.exit(main())
