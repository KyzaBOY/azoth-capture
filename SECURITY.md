# Security

Found something that looks wrong, or something this app does that the
[README](README.md) says it doesn't? Please tell us.

- Preferred: open a **private security advisory** on this repository
  (Security tab → Report a vulnerability).
- Or ask in the Azoth Discord and mention that it is about Azoth Capture.

Please include the version (shown in the app and in `wails.json`), your game
build (shown in the Activity Log when you press Start), and what you saw.

What is in scope: this repository's code, the installer script
(`internal/capture/embed/install_env.bat`), and the release binaries. The
upstream capture engine is Coldzer0's project; if the problem is in it, report
it there as well and we will pin the installer to a safe commit.
