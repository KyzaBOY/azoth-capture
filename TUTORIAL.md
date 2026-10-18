# Azoth Capture — How to Record Game Data

> Keep this guide up to date with the app.
> Written for people who have never touched packet capture before.

---

## What is this?

**Azoth Capture** is a single `.exe` that records everything the New World
server sends to your game client — every item you pick up, every mob you
kill, every step you take. We use these recordings to rebuild the server
from scratch before Amazon shuts it down on **January 31, 2027**.

It wraps [Aeternum-World's nw_capture](https://github.com/Coldzer0/Aeternum-World)
by Coldzer0, with a built-in **journal** that timestamps your actions so we
know exactly what happened at each moment in the capture. No video recording
needed — just press a button.

---

## Quick start (under 2 minutes to learn)

1. **Download** `AzothCapture.exe` from the
   [Releases page](https://github.com/KyzaBOY/azoth-capture/releases) — it's a
   single file. Put it in a folder of its own (it creates its helper files
   next to itself). No installer, no admin rights. The release notes list
   its SHA-256 (`SHA256SUMS.txt`), and the whole source is in this repository; see the
   [README](README.md) if you want to check it or build it yourself.

2. **Open it.** On first launch it looks like this: a clock at the top,
   a Session panel below, and an activity log at the bottom.

3. **Set the game path.** Click **Browse** next to "Game executable" and
   find your `NewWorld.exe`:
   ```
   C:\SteamLibrary\steamapps\common\New World\Bin64\NewWorld.exe
   ```
   The app remembers this — you only do it once.

4. **Pick a session name** (or leave the default). Choose an **Activity**
   from the dropdown that matches what you plan to do (Fishing, Combat,
   Crafting, etc.).

5. **Click Start Session.** That's it. The app:
   - Installs Python and the capture tools automatically (first time only,
     takes 2–3 min — progress shows in the Activity Log)
   - Launches New World **without Easy Anti-Cheat**
   - Starts recording all network traffic
   - Starts the journal timer

6. **Play the game.** Every time you do something meaningful (equip an
   item, cast a line, kill a mob), press the **Mark** button or hit
   **F8** (works while the game has focus). Type a short note of what
   you just did — "equipped iron pickaxe", "caught a fish", "died to wolf".

7. **When you're done**, Alt+F4 the game and click **Stop Session** in the
   app. It saves everything automatically.

8. **Post your capture.** When you stop, the app builds a `send` folder
   containing only the two files we need — drop them where the Azoth
   Discord tells you to:
   - `ledger.bin` (the main capture file)
   - `steps.json` (your journal — this tells us what you did and when)
   - A short message saying what activity you focused on

---

## The EAC situation (read this)

The capture tool launches `NewWorld.exe` directly via Frida, **bypassing
the normal launcher chain** (Steam → EAC → Game). This means Easy
Anti-Cheat never loads. It's the only way to intercept the network data.
Here's what you need to know:

- **Ban risk: none reported to us, but we can't promise.** Running the game
  without EAC is against the game's rules in principle. We have not heard of
  anyone being banned for it: the game connects to the official server
  normally, it just runs without EAC alongside it, and other communities
  (Aeternum-World, first-light, NewWorldPreservation) have been doing this
  routinely. Amazon could still act on it; if that worries you, use an
  account you can afford to lose. You are the one deciding to run it.
- **You WILL get disconnected every 2–3 minutes.** The server notices EAC
  isn't responding and kicks you. This is normal and expected — it's not
  a ban, just a timeout.
- **The capture keeps running.** When you get kicked, just log back in.
  The tool continues recording across reconnections. Each disconnect/
  reconnect is captured too, which is actually useful data for us.
- **Steam must be running.** The tool writes a `steam_appid.txt` so
  SteamAPI initializes correctly, but Steam itself needs to be open and
  logged in.
- **Plan around the timeouts.** You have about 2–3 minutes per window
  before a kick. Do one focused action per window:
  - Log in → equip an item → get kicked → log back in → craft something
    → get kicked → etc.
  - Press **Mark** (or F8) before and after each action so we know the
    exact timestamps.

### Timeout setting

The **Timeout** field in the app controls how long the capture tool runs
before it kills the game process automatically.

- **600** (default) = 10 minutes. Gives you 3–5 reconnection cycles,
  enough for most focused tasks.
- **1800** = 30 minutes. Good for longer sessions (city replication,
  extended combat).
- **3600** = 1 hour. For marathon capture sessions.
- You can always stop manually with **Stop Session** — the timeout is
  just a safety net so the game doesn't run forever if you forget.

### One per endpoint

- **Off (default):** every HTTPS request is saved. This is what you want
  for most captures.
- **On:** keeps only the first request per (verb, host, path). Useful
  for building a clean catalog of endpoints without duplicates, but
  loses the repeated auth refreshes. Leave it off unless someone asks
  for it.

---

## What we need (and how you help)

Check the **Missions** panel in the app (right side) — it loads the
community's current needs from `projectazoth.com` (one plain GET request, no
data about you is sent). Packets marked **wanted** are the ones nobody has
captured yet.

### Priority list (most needed first)

**Inventory & items:**
- Moving items between slots (drag from one slot to another)
- Splitting and merging stacks
- Dropping an item on the ground and picking it back up
- Using a potion / food (from inventory and from the quick slot bar)
- Swapping weapons (1 → 2) and sheathing
- Equipping / removing a bag, going light → medium → heavy load
- Equipping jewelry and gathering tools

**Combat & world:**
- Gathering a node (mining, harvesting, logging) — with trade skill XP
- Killing a mob and looting it
- Taking damage, dying, and respawning

**Replication (advanced, very valuable):**
- Standing ~1 min in a busy town with other players and NPCs moving around
- Another player equipping gear or swapping weapons near you

### How to do a good capture

1. **One action group per session.** Don't try to capture everything in
   one go. "Fishing session", "inventory moves session", "combat session".
2. **One action at a time, with ~5 second pauses** between them. This
   makes it much easier to isolate each action in the data.
3. **Mark every action.** Press F8 before you do something, then rename
   the step to describe it. The timestamps are what make your capture
   10x more valuable than a raw dump.
4. **Use the checklist.** If you picked a mission from the Missions
   panel, the app shows step-by-step instructions. Click each step as
   you do it — it marks it in the journal automatically.

---

## Why the journal matters

Without the journal, a capture is a wall of binary data and someone has
to watch a video (if you recorded one) or guess what happened when.

With the journal, we get `steps.json`:
```json
[
  {"t_ms": 1729872345123, "text": "equipped iron pickaxe", "via": "button"},
  {"t_ms": 1729872351456, "text": "mined iron vein",       "via": "hotkey"},
  {"t_ms": 1729872358789, "text": "picked up ore",         "via": "hotkey"}
]
```

Each step has a **millisecond-accurate timestamp** synced to the same
clock the capture tool uses. We can jump straight to the exact packet
that corresponds to "equipped iron pickaxe" instead of scrubbing through
thousands of packets. This is what makes the difference between a capture
we can use in a day vs. one that sits in a folder for weeks.

**You don't need to record video.** The journal replaces it. Just mark
your actions and write short descriptions.

---

## What NOT to share

> **Never share the HTTPS folders, keylog.txt, or raw log files.**
> They contain your Amazon/Steam account session tokens, Steam tickets,
> and TLS key material that would let anyone decrypt your traffic.

**What we ask you to share (the `send` folder):**
- `ledger.bin` — the decrypted game-protocol traffic, which is what we need.
  It shows what your character did, and in our own captures it also carries
  the game's voice-chat (Vivox) session data. That is session data of your
  account, so share it only if you are comfortable with that.
- `steps.json` — your journal (your notes and timestamps)

**NEVER share:**
- `meta.json` — it contains full paths on your PC, including your Windows
  user name
- `https/` folder — contains auth tokens, cookies, session identifiers
  for your Amazon and Steam accounts
- `keylog.txt` — TLS key material (lets anyone decrypt the raw capture)
- `logs/` folder — raw JSONL event stream with headers and credentials
- `summary.json` — contains your local file paths

The app's `send` folder holds only `ledger.bin` and `steps.json`. Share
those, nothing else from the capture folder.

---

## Troubleshooting

**"Setting up..." takes forever**
The first-time install downloads ~200 MB (Python, Git, the capture repo,
and Python packages like Frida). On a slow connection this can take 5–10
minutes. Check the Activity Log for progress. Once installed, future
sessions start instantly.

**"This game build is not supported by this version of Azoth Capture"**
Every game patch moves the code Azoth Capture hooks. The app finds it by
its machine code instead of by fixed addresses, so a patch that only shifts
code just works, but if a patch changes those functions the app refuses to start rather than
crash your game. Nothing was launched. Get the newest release and, if there
is none yet, tell us the build number shown in the Activity Log.
Hooks located and installed on: **1.460.6033.6024067** (a full live capture
on this build is still being confirmed).

**Game doesn't launch**
Make sure the path points to `NewWorld.exe` inside `Bin64\`, not a
shortcut or the launcher. The exact path is usually:
```
C:\SteamLibrary\steamapps\common\New World\Bin64\NewWorld.exe
```

**Kicked immediately (< 30 seconds)**
The 2–3 minute EAC timeout is normal, but if you're kicked in under 30
seconds, the server might be having issues. Try a different world/server.

**"Capture ended with error"**
Check the Activity Log for details. Common causes:
- Game closed before the capture tool could attach (restart the session)
- Python package failed to install (delete the `python_embedded` folder
  next to the exe and restart — it will reinstall)

**F8 hotkey doesn't work**
Another app might be using F8. Click **Change** in Settings and pick a
different key (F9, F10, Ctrl+F8, etc.).

---

## File structure

After a capture session, the files are saved in two places:

**Capture data** (next to the exe):
```
AzothCapture/
├── Aeternum-World/Tools/nw_capture/captures/
│   └── your-session-name/
│       ├── dtls/
│       │   ├── ledger.bin      ← THE main file (DTLS game packets)
│       │   ├── keylog.txt      ← DO NOT SHARE (TLS keys)
│       │   └── meta.json       ← DO NOT SHARE (has local paths)
│       ├── tokenservice/       ← DO NOT SHARE
│       ├── gateway_cf/         ← DO NOT SHARE
│       ├── (other HTTPS dirs)  ← DO NOT SHARE
│       └── summary.json        ← DO NOT SHARE (has local paths)
```

**Journal export** (in your Documents):
```
Documents/Azoth Captures/
└── your-session-name/
    ├── steps.json          ← structured journal with timestamps
    └── steps.txt           ← human-readable version
```

**What to post:** the two files in the `send` folder, `ledger.bin` +
`steps.json`, and nothing else.

---

## The clock is ticking

Amazon shuts down New World: Aeternum on **January 31, 2027**. There is no
offline mode, no private servers, no plan B from Amazon. After that date,
no new captures can ever be made — we can only work with what we have.

Every capture you record today is permanent. The server goes away, but
the data you recorded lives on and lets us rebuild it. Even a 5-minute
fishing session with journal annotations is more valuable than nothing.

The **Missions** panel shows what is still missing. Any action it lists
that you record is a first.

---

## Credits

- **Capture engine:** [Aeternum-World](https://github.com/Coldzer0/Aeternum-World)
  by Coldzer0 (nw_capture, Frida-based DTLS/HTTPS interception)
- **Original GUI:** NW Capture App by Potz (MIT license)
- **Azoth Capture:** rebuilt from scratch for the Azoth preservation project
  (Go + Wails, journal integration, auto-setup)

No telemetry. No accounts. Apart from one plain request for the Missions
list, nothing leaves your machine except what you choose to post.
