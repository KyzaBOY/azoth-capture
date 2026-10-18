# Third-party software

Azoth Capture's own code is MIT (see `LICENSE`). It builds on, and at run time
fetches, the following. None of the upstream code below is copied into this
repository (the tests use made-up stand-ins).

| Component | License | How it is used |
|---|---|---|
| [Aeternum-World](https://github.com/Coldzer0/Aeternum-World) by Coldzer0 (`Tools/nw_capture`) | AGPL-3.0 | The capture engine. Cloned on the user's machine by the installer, pinned to commit `820156dbc44c86c9436af81aa0dba72e94cb636b`, and run as a separate Python process, with one script swapped for the patched copy below. |
| Patched copy of its `_dtls_ledger.js` | AGPL-3.0 (derivative of the above) | Generated on the user's machine at launch, never distributed by this repo. Only the table of hook addresses differs; see `internal/capture/agent.go`. Corresponding source: the upstream repo plus that function. |
| NW Capture App by Potz | MIT | Inspiration for the original GUI. No code from it is included. |
| [Frida](https://frida.re) | wxWindows Library Licence | Installed from PyPI by the installer. |
| Python 3.12 (embeddable) | PSF | Downloaded from python.org by the installer. |
| Git for Windows (portable) | GPL-2.0 | Downloaded from github.com/git-for-windows by the installer. |
| [Wails](https://wails.io) v2 | MIT | The desktop shell (compiled into the app). |
| golang.org/x/sys | BSD-3-Clause | Compiled into the app. |
| IBM Plex Mono, Space Grotesk | SIL OFL 1.1 | Fonts in `frontend/fonts` (license texts alongside). |

The license texts of the Go libraries compiled into the exe are in
`third_party/licenses` (generated from the packages the app actually links with
`go-licenses` v1.6.0).

"New World" and "New World: Aeternum" are trademarks of Amazon. This project is
not affiliated with or endorsed by Amazon, and contains no Amazon code or
assets.
