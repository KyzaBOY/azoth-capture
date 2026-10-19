"""Azoth Capture attach helper (Linux).

Usage (done by the app, not by hand):
    python azoth_attach.py --nw-dir <nw_capture dir> --ledger-js <patched _dtls_ledger.js>
                           --sites <hook sites JSON> --host <ip:port> --session <name>
                           --captures-root <dir> [--timeout <seconds>]

What it does: connects to a Windows Frida server running inside the game's
Proton prefix, finds the one running NewWorld.exe, and ATTACHES to it. It first
checks that the code at each hook site in the game's memory is exactly the code
the app scanned in the executable file; if any site differs, nothing is hooked.
Then it loads the upstream Aeternum-World helpers and DTLS ledger unchanged (the
real hooks live there) and writes the decrypted session into
<captures-root>/<session>/dtls/ledger.bin, the same layout the Windows capture
produces. It never spawns or kills the game.

It deliberately does NOT load the upstream HTTPS tap (nw_https_tap.js). That
script swaps one of the game's WinHTTP callback pointers for its own. On Windows
the capture closes the game at the end, so that is harmless; here the game keeps
running after Stop, and the swapped pointer would then point at freed memory.
The tap only feeds the HTTPS extract, which the Linux capture does not make.

Why attach instead of spawn: on Linux the game is launched the normal way through
Steam, so Easy Anti-Cheat stays active and the server does not disconnect the
player. Attaching from a Frida server inside the same Proton prefix reads the
traffic without taking the anti-cheat offline.

On SIGINT or SIGTERM (the app's Stop) it flushes the last data, detaches from
the game and stops the Frida server, then exits.
"""
import argparse
import json
import os
import re
import signal
import threading
import time
from datetime import datetime, timezone
from pathlib import Path

import frida

GAME_PROCESS = "NewWorld.exe"
SERVER_PROCESS = "frida-server.exe"
PRIVATE_UMASK = 0o077  # the key log holds TLS secrets: owner-only files
LEDGER_FLUSH_SECONDS = 1.0
SITE_CHECK_SECONDS = 10.0  # how long to wait for the in-memory check verdict
SITE_NAMES = ("SSL_read", "SSL_write", "nss_keylog")
SITE_VA = re.compile(r"0x[0-9A-F]+")
SITE_BYTES = re.compile(r"[0-9A-F]{2}( [0-9A-F]{2}){15}")  # 16 bytes, nwHexdump style

# Runs before the ledger arms its hooks. It compares the first 16 bytes of each
# hook site in memory with the bytes from the executable file and throws if any
# differs or is not executable code. A throw stops the script there, so the
# ledger below never runs and nothing is hooked.
SITE_GUARD = """
(function () {
    const sites = %s;
    nwInit();
    for (const site of sites) {
        const addr = nwRva(site.va);
        const range = Process.findRangeByAddress(addr);
        if (range === null || !range.protection.includes('x')) {
            throw new Error('azoth: ' + site.name + ' is not in executable memory');
        }
        const found = nwHexdump(addr, 16);
        if (found !== site.bytes) {
            throw new Error('azoth: ' + site.name + ' in memory is ' + found
                            + ', expected ' + site.bytes);
        }
    }
    nwSend('azoth_sites_verified', { count: sites.length });
})();
"""

# Appended to the agent so the helper can flush the ledger's staging buffer on
# Stop. The upstream ledger exposes its flush on NW but not as an RPC export.
FLUSH_EXPORT = """
rpc.exports.azothFlush = function () {
    if (typeof NW !== 'undefined' && NW._dtls_ledger_flush) NW._dtls_ledger_flush();
};
"""


def parse_sites(text: str) -> list:
    """Validate the hook-site list the app passes in. It is embedded in agent
    code, so anything but the exact expected shape is refused."""
    sites = json.loads(text)
    if (not isinstance(sites, list)
            or [s.get("name") if isinstance(s, dict) else None for s in sites] != list(SITE_NAMES)):
        raise ValueError(f"hook sites must be {', '.join(SITE_NAMES)}, in that order")
    for s in sites:
        if set(s) != {"name", "va", "bytes"}:
            raise ValueError(f"hook site {s.get('name')}: unexpected fields")
        if not (isinstance(s["va"], str) and SITE_VA.fullmatch(s["va"])):
            raise ValueError(f"hook site {s['name']}: bad address")
        if not (isinstance(s["bytes"], str) and SITE_BYTES.fullmatch(s["bytes"])):
            raise ValueError(f"hook site {s['name']}: bad bytes")
    return sites


def build_agent(nw_dir: Path, ledger_js: Path, sites: list) -> str:
    common = nw_dir / "_common.js"
    for path in (common, ledger_js):
        if not path.is_file():
            raise FileNotFoundError(path)
    # Helpers first, then the in-memory check, then the self-arming DTLS ledger
    # (this is what fills ledger.bin), then our flush export.
    guard = SITE_GUARD % json.dumps(sites)
    return "\n".join([common.read_text(encoding="utf-8"), guard,
                      ledger_js.read_text(encoding="utf-8"), FLUSH_EXPORT])


class Attach:
    def __init__(self, args):
        self.host = args.host
        self.timeout = float(args.timeout)
        self.session_name = args.session
        self.agent_src = build_agent(Path(args.nw_dir).resolve(),
                                     Path(args.ledger_js).resolve(),
                                     parse_sites(args.sites))

        dtls = Path(args.captures_root).resolve() / self.session_name / "dtls"
        dtls.mkdir(parents=True, exist_ok=True)
        self.ledger_path = dtls / "ledger.bin"
        self.keylog_path = dtls / "keylog.txt"
        self.meta_path = dtls / "meta.json"
        self.ledger_f = None
        self.keylog_f = None

        self.started_at = datetime.now(timezone.utc).isoformat()
        self.ledger_bytes = 0
        self.ledger_batches = 0
        self.keylog_lines = 0
        self.hooks = None
        self.sites_ok = False
        self.load_error = None
        self.verdict = threading.Event()  # set when the in-memory check answers
        self.lock = threading.Lock()
        self.stop = threading.Event()
        self._last_flush = time.time()

    def run(self) -> int:
        for sig in (signal.SIGINT, signal.SIGTERM):
            signal.signal(sig, lambda *_: self.stop.set())

        device = frida.get_device_manager().add_remote_device(self.host)
        try:
            return self._capture(device)
        finally:
            stop_server(device)

    def _capture(self, device) -> int:
        params = device.query_system_parameters()
        if params.get("platform") != "windows" or params.get("arch") != "x64":
            print("-- the capture server is not Windows x64; refusing to attach")
            return 2

        pids = process_ids(device, GAME_PROCESS)
        if len(pids) != 1:
            print(f"-- expected exactly one {GAME_PROCESS}, found {len(pids)}")
            return 2
        pid = pids[0]
        print(f"-- attaching to {GAME_PROCESS} (pid {pid})")

        self.ledger_f = self.ledger_path.open("ab")
        self.keylog_f = self.keylog_path.open("a", encoding="ascii")
        print(f"-- ledger: {self.ledger_path}")

        fsession = device.attach(pid)
        fsession.on("detached", lambda *_: self.stop.set())
        script = fsession.create_script(self.agent_src)
        script.on("message", self._on_message)
        # load() returns even when the script throws; the in-memory check's
        # verdict arrives as a message, so wait for it before going on.
        script.load()
        self.verdict.wait(timeout=SITE_CHECK_SECONDS)
        if not self.sites_ok:
            reason = self.load_error or "no answer from the in-memory check"
            print(f"-- hook sites do not match this game in memory ({reason}); nothing was hooked")
            self._abandon(script, fsession)
            return 2
        print("-- hook sites match the game in memory")

        print("-- recording; stop from the app, or wait for the timeout")
        self.stop.wait(timeout=self.timeout or None)

        self._finish(script, fsession)
        return 0

    def _on_message(self, message: dict, data) -> None:
        if message.get("type") == "error":
            print("-- agent error:", message.get("description"))
            if not self.sites_ok:
                self.load_error = message.get("description")
                self.verdict.set()
            return
        if message.get("type") != "send":
            return
        payload = message.get("payload")
        if not isinstance(payload, dict):
            return
        kind = payload.get("type")
        if kind == "azoth_sites_verified":
            self.sites_ok = True
            self.verdict.set()
            return
        with self.lock:
            if self.ledger_f is None:
                return  # sinks already closed during shutdown
            if kind == "dtls_ledger" and data is not None:
                self.ledger_f.write(data)
                self.ledger_bytes += len(data)
                self.ledger_batches += 1
                now = time.time()
                if now - self._last_flush > LEDGER_FLUSH_SECONDS:
                    self.ledger_f.flush()
                    self._last_flush = now
            elif kind == "dtls_keylog":
                self.keylog_f.write(
                    f'{payload.get("label", "")} '
                    f'{payload.get("cr_hex", "")} '
                    f'{payload.get("sec_hex", "")}\n')
                self.keylog_f.flush()
                self.keylog_lines += 1
            elif kind == "dtls_hook_installed":
                self.hooks = {k: payload.get(k) for k in
                              ("ssl_read", "ssl_write", "nss_keylog")}
                print("-- hooks armed:", self.hooks)

    def _abandon(self, script, fsession) -> None:
        """Leave without recording: detach, close the sinks, and remove the
        output files this run created if they are still empty."""
        self._detach(script, fsession)
        self._close_sinks()
        for path in (self.ledger_path, self.keylog_path):
            if path.exists() and path.stat().st_size == 0:
                path.unlink()

    def _detach(self, script, fsession) -> None:
        for closer in (script.unload, fsession.detach):
            try:
                closer()
            except frida.InvalidOperationError:
                pass  # already gone (the game exited or detached)

    def _close_sinks(self) -> None:
        with self.lock:
            for sink in (self.ledger_f, self.keylog_f):
                sink.flush()
                sink.close()
            self.ledger_f = self.keylog_f = None

    def _finish(self, script, fsession) -> None:
        # Flush the agent's staging buffer first, so the last records arrive
        # before the hooks are removed.
        try:
            script.exports_sync.azoth_flush()
        except (frida.core.RPCException, frida.InvalidOperationError) as e:
            print(f"-- note: final flush skipped: {e}")
        self._detach(script, fsession)
        self._close_sinks()
        meta = {
            "session": self.session_name,
            "started_at_utc": self.started_at,
            "stopped_at_utc": datetime.now(timezone.utc).isoformat(),
            "mode": "attach",
            "hook_sites_verified": self.sites_ok,
            "ledger_bytes": self.ledger_bytes,
            "ledger_batches": self.ledger_batches,
            "keylog_lines": self.keylog_lines,
            "hooks": self.hooks,
        }
        self.meta_path.write_text(json.dumps(meta, indent=2))
        print()
        print("-- capture summary")
        print(f"   ledger bytes   = {self.ledger_bytes}")
        print(f"   ledger batches = {self.ledger_batches}")
        print(f"   keylog lines   = {self.keylog_lines}")


def process_ids(device, name: str) -> list:
    return [p.pid for p in device.enumerate_processes()
            if p.name.lower() == name.lower()]


def stop_server(device) -> None:
    """Stop the Frida server inside the prefix, so nothing of ours stays
    running next to the game after the capture."""
    try:
        for pid in process_ids(device, SERVER_PROCESS):
            device.kill(pid)
    except (frida.TransportError, frida.ServerNotRunningError,
            frida.ProcessNotFoundError, frida.InvalidOperationError):
        pass  # the server is already gone, or went away while stopping


def main() -> int:
    os.umask(PRIVATE_UMASK)
    parser = argparse.ArgumentParser()
    parser.add_argument("--nw-dir", required=True)
    parser.add_argument("--ledger-js", required=True)
    parser.add_argument("--sites", required=True)
    parser.add_argument("--host", required=True)
    parser.add_argument("--session", required=True)
    parser.add_argument("--captures-root", required=True)
    parser.add_argument("--timeout", default="0")
    args = parser.parse_args()
    try:
        return Attach(args).run()
    except FileNotFoundError as e:
        print(f"-- missing file: {e}")
        return 2
    except ValueError as e:
        print(f"-- bad hook-site list from the app: {e}")
        return 2
    except (frida.ServerNotRunningError, frida.TransportError) as e:
        print(f"-- the capture server is not reachable: {e}")
        return 2
    except frida.ProcessNotFoundError as e:
        print(f"-- the game went away before the capture could attach: {e}")
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
