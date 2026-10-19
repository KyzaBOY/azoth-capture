# Azoth Capture on Linux

Azoth Capture runs on Linux as well as Windows. The window, the journal, the F8
mark key and the two files you post are the same. What changes is how it
records. For step-by-step use, see
[Playing on Linux](../TUTORIAL.md#playing-on-linux) in the tutorial; this page
covers setup, building, and what the app does under the hood.

## How it records, and why that should stop the kicks

On Windows the app starts the game with Easy Anti-Cheat (EAC) switched off,
which is why the server disconnects you every 2 to 3 minutes.

On Linux you start the game the normal way through Steam, so EAC stays on. The
app then attaches to the running game instead of starting it without EAC. In
testing, a 14-minute session stayed on one connection the whole time, with
none of the Windows kicks. That is one test on one PC, so take it as a good
sign rather than a promise.

The game runs under Proton as usual. A small Windows
program, the Frida server, runs inside the game's Proton prefix. Because it
lives in the same place as the game, it can read the game's data. The app
attaches to the game through that server and writes the same `ledger.bin` a
Windows capture writes.

Running capture tooling against the live game is against the game's rules in
principle, on Linux as on Windows. Keeping EAC on is gentler than the Windows
method, but it is not a promise. Use an account you are comfortable using.

## What it does inside the game

The app keeps what it does inside the running game small.

Before it places any hook, it checks that the code at each of the three hook
sites in the game's memory matches, byte for byte, the code it found in
`NewWorld.exe` on disk. If any site differs, it hooks nothing and tells you,
rather than risk crashing the game.

It then hooks only those three functions (the DTLS read, write and key log)
through the upstream ledger script. That is all `ledger.bin` needs.

It does not load the upstream HTTPS script. That script swaps one of the game's
internal callback pointers for its own. On Windows the capture closes the game
at the end, so that does no harm, but on Linux the game keeps running after you
stop, and the swapped pointer would then point at freed memory.

When you stop, it flushes the last data, removes its hooks, detaches, and shuts
down its Frida server. The game keeps running untouched.

## What you need

You need Steam and New World installed, with the game able to launch and play
under Proton. Use whichever Proton version runs the game on your system
(Properties, then Compatibility, in Steam). Which one works differs from
machine to machine: one system runs it on Proton 11, another only on GE-Proton
10. The app reads the version the game is set to use from Steam and uses that
same one. Steam installs the Steam Linux Runtime it needs along with Proton.

You also need `python3` with `venv`, `git`, `curl`, `xz` and `sha256sum`.
Almost every distribution has them, and the app tells you if one is missing.
On Debian and Ubuntu, `venv` comes in the separate `python3-venv` package.

Two things are optional. The global F8 key needs permission to read input
devices (see [The F8 mark key](#the-f8-mark-key)); the on-screen Mark button
works without it. Video needs GPU Screen Recorder (see
[Video](#video-optional-off-by-default)).

You don't need to know where anything is installed. The app finds the game,
the Proton version and the runtime by asking Steam, so it works on any distro
and any library folder. It looks for the usual Steam install
(`~/.local/share/Steam` or `~/.steam`) and the Flatpak one. The Flatpak and
Snap versions of Steam haven't been tested yet.

## Video (optional, off by default)

The app can also record video of your session, but only if you turn it on. It
is off on every new install and never starts a clip by itself.

Video uses GPU Screen Recorder. Install it once from your distribution (on
Debian and Ubuntu: `sudo apt install gpu-screen-recorder`) or install
`com.dec05eba.gpu_screen_recorder` from Flathub. Then press **Check again**
next to Video in the app's Settings.

Once you turn video on, three settings appear.

The screen setting picks what to record. With New World open, press **Pick
window**, and your desktop shows its own "choose what to share" dialog; choose
the New World window. You only do this once. Later clips record that window
without asking, on whichever monitor it is. Press **Pick window** again to
choose a different one. Desktops without that dialog, such as X11 sessions,
ask you to choose the monitor the game is on instead; the app never picks one
for you.

The sound setting is "Game sound, no microphone" or "No sound". Game sound is
only what New World itself plays. Your microphone is never recorded: the app
never asks for a microphone device, and the recorder's per-program sound only
takes what a program plays, never anything it records. What the game plays
does include other players' voice chat, so choose "No sound" if you don't want
that in your clips.

The video key starts and stops a clip. It is F9 unless you change it, and like
F8 it needs the `input` group. The **Start video** button works without it.

Video is separate from the session. Starting a session doesn't start video;
you start each clip yourself, and each start makes a new clip. Video also
never runs without the capture. A clip can't start while the capture isn't
running, and a running clip is stopped and saved whenever the capture ends,
whatever the reason: Stop Session, the timeout, the game closing, or an error.

Clips go into the session folder as
`~/Documents/Azoth Captures/<session>/gameplay-001.mkv`, `-002.mkv` and so on,
with a `video.json` next to them. For each clip, `video.json` records when its
first frame was captured, on the same PC clock as `ledger.bin` and
`steps.json`, so a clip lines up with the packets and your marks.

Video never goes into the `send` folder. It can show your character name, chat
and anything else on screen, so it stays on your PC unless you choose to share
it.

Clips are scaled to fit 1920x1080 at 60 fps. Expect a few GB per hour.

Picking a window needs a desktop with the screen-cast portal, which KDE Plasma
and GNOME on Wayland have. If a clip shows the wrong picture, press **Pick
window** again and choose the New World window.

## The F8 mark key

The global mark key works on X11 and Wayland, and while the game has focus. To
do that, it reads the keyboard straight from the kernel, and you should know
what that involves.

Unlike on Windows, the kernel hands every key press to the app, not just F8.
The app keeps only the state of the modifier keys and the moment your chosen
key goes down. It drops every other key press at once and never stores, logs
or sends it. The code is in `internal/hotkey/hotkey_linux.go`.

Most desktops don't let normal programs read the keyboard this way. To allow
it, you add your user to the `input` group, and that permission isn't limited
to this app: every program you run can then read the keyboard. Decide whether
you're comfortable with that. To turn it on:

```bash
sudo usermod -aG input "$USER"
```

Then log out and back in. If you'd rather not, skip it. The on-screen Mark
button does the same job, and the app tells you the global key is off.

## Where files go

Settings live in `~/.config/AzothCapture/`. Exported sessions go to
`~/Documents/Azoth Captures/` (or your desktop's Documents folder). The capture
tools and the Python environment sit next to the app in their own folder;
delete that folder and they're gone.

## Building

A Linux build needs Go 1.26 or newer, a C compiler, and the GTK3 and
WebKit2GTK development packages, which Wails uses for the window. Install them
with your distribution's package manager, for example:

- Debian and Ubuntu: `build-essential libgtk-3-dev libwebkit2gtk-4.1-dev`
- Fedora: `gtk3-devel webkit2gtk4.1-devel`
- Arch: `gtk3 webkit2gtk-4.1`

Install the Wails command:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

Then build from the repository root. Wails looks for WebKitGTK 4.0 by default,
but most current distributions only ship 4.1 (`libwebkit2gtk-4.1`). On those,
add the `webkit2_41` tag:

```bash
wails build -platform linux/amd64 -tags webkit2_41
```

With plain `go build`, use `-tags "desktop,production,webkit2_41"`. Leave the
tag out only if your system has the older WebKitGTK 4.0 packages.

## What it downloads

On first start it downloads, over HTTPS and into the app's own folder:

| What | From | Integrity |
|---|---|---|
| Capture tools: [Aeternum-World](https://github.com/Coldzer0/Aeternum-World) | `github.com/Coldzer0/Aeternum-World` | pinned to one fixed commit |
| Python packages (Frida and others) | PyPI | exact versions from the tools' `requirements.txt` |
| Frida server (Windows build, run inside the Proton prefix) | `github.com/frida/frida` releases | version pinned and SHA-256 verified |

Python itself and Git come from your system and are not downloaded.
