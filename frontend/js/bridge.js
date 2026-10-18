(function () {
  "use strict";

  const params = new URLSearchParams(location.search);
  const real = !!(window.go && window.go.main && window.go.main.App);

  const listeners = {};
  const bus = {
    on(name, cb) {
      if (window.runtime && window.runtime.EventsOn) return window.runtime.EventsOn(name, cb);
      (listeners[name] = listeners[name] || []).push(cb);
      return () => { listeners[name] = (listeners[name] || []).filter((f) => f !== cb); };
    },
    emitLocal(name, data) {
      (listeners[name] || []).forEach((cb) => { try { cb(data); } catch (e) { console.error(e); } });
    },
  };

  const win = {
    minimise() { if (window.runtime && window.runtime.WindowMinimise) window.runtime.WindowMinimise(); },
    quit() { if (window.runtime && window.runtime.Quit) window.runtime.Quit(); else console.log("[mock] Quit"); },
  };

  if (real) {
    window.CaptureBridge = { real: true, api: window.go.main.App, bus, win, params };
    return;
  }

  const ACTIVITIES = ["Fishing", "Combat", "Inventory & gear", "Chat", "Trading post", "Housing",
    "Expeditions", "Wars & PvP", "Crafting", "Gathering", "Quests", "Login & world entry",
    "Movement", "Social & groups", "Companies", "Progression", "Store & cosmetics", "Camping", "Other"];

  let now = () => Date.now();
  let S = {
    schema: 1, phase: "idle", name: "", activity: "", started_ms: 0, stopped_ms: 0, targets: [], steps: [],
    next_id: 1, exported: false, saved_ms: 0, export_dir: "", draft_error: "",
  };
  const set = { hotkey: "F8", always_on_top: false, output_dir: "C:\\Users\\you\\Documents\\Azoth Captures", exe_path: "", timeout: 600, one_per_endpoint: false };
  const emit = () => bus.emitLocal("state", JSON.parse(JSON.stringify(S)));
  const snap = () => JSON.parse(JSON.stringify(S));
  const slug = (a) => a.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  const clean = (t) => (t || "").replace(/\s+/g, " ").trim().slice(0, 500);
  const mark = (via, text, target) => {
    if (S.phase !== "running") return Promise.reject("start a session first");
    const t = now();
    const st = { id: S.next_id++, t_ms: t, text: clean(text) || "Step " + (S.steps.length + 1), via };
    if (target !== undefined && target !== null) st.target = target;
    S.steps.push(st);
    S.steps.sort((a, b) => a.t_ms - b.t_ms);
    S.saved_ms = t;
    emit();
    return Promise.resolve(snap());
  };

  if (params.get("run") === "1") {
    const t0 = now() - 5 * 60000;
    S = Object.assign(S, { phase: "running", name: "fishing-01", activity: "Fishing", started_ms: t0, targets: [], next_id: 3, saved_ms: now() - 2000 });
    S.steps = [
      { id: 1, t_ms: t0 + 8000, text: "Equipped the fishing pole", via: "button" },
      { id: 2, t_ms: t0 + 41000, text: "Cast the line", via: "hotkey" },
    ];
  }

  const needsInstall = params.get("installed") === "0";

  const api = {
    Boot: () => Promise.resolve({
      version: "0.1.0", settings: set, activities: ACTIVITIES, hotkey_error: "",
      draft: { found: false }, state: snap(), env_ready: !needsInstall,
      app_dir: "C:\\Tools\\AzothCapture",
    }),
    SuggestName: (a) => Promise.resolve(slug(a || "other") + "-01"),
    StartSession: (exe, name, activity, timeout, onePerEP) => new Promise((resolve) => {
      if (needsInstall) {
        const msgs = ["First-time setup — downloading Python and capture tools...", "This only happens once. Sit tight.", "",
          "Downloading Python 3.12.9...", "Extracting Python...", "Installing pip...",
          "Downloading portable Git...", "Cloning Aeternum-World...", "Installing packages...",
          "", "Environment setup complete!", ""];
        let i = 0;
        const iv = setInterval(() => {
          bus.emitLocal("caplog", msgs[i++]);
          if (i >= msgs.length) {
            clearInterval(iv);
            bus.emitLocal("capstatus", "installed");
            startBoth();
          }
        }, 200);
      } else {
        startBoth();
      }
      function startBoth() {
        bus.emitLocal("capstatus", "running");
        bus.emitLocal("caplog", "Starting capture: " + name);
        bus.emitLocal("caplog", "Target: " + exe);
        let i = 0;
        setInterval(() => {
          bus.emitLocal("caplog", '{"ts_ms": ' + Date.now() + ', "type": "winhttp_read_data", "tid": ' + (30000 + i) + '}');
          i++;
        }, 2000);
        S = Object.assign(S, { phase: "running", name, activity, started_ms: now(), stopped_ms: 0, steps: [], next_id: 1, export_dir: "" });
        emit();
        resolve(snap());
      }
    }),
    StopSession: () => {
      S.phase = "stopped"; S.stopped_ms = now(); S.export_dir = set.output_dir + "\\" + S.name;
      const sendDir = S.export_dir + "\\send";
      bus.emitLocal("capstatus", "done");
      bus.emitLocal("caplog", "Capture stopped by user.");
      emit();
      return Promise.resolve({ state: snap(), export_dir: S.export_dir, export_error: "", send_dir: sendDir, size_warning: "" });
    },
    MarkButton: () => mark("button"),
    MarkChecklist: (text, idx) => mark("checklist", text, idx),
    EditStep: (id, text) => { const s = S.steps.find((x) => x.id === id); if (s) s.text = clean(text) || s.text; emit(); return Promise.resolve(snap()); },
    DeleteStep: (id) => { S.steps = S.steps.filter((x) => x.id !== id); emit(); return Promise.resolve(snap()); },
    SetTargets: (ids) => { S.targets = Array.from(new Set(ids)); emit(); return Promise.resolve(snap()); },
    NewSession: () => { S = Object.assign(S, { phase: "idle", name: "", steps: [], targets: [], export_dir: "", started_ms: 0, stopped_ms: 0 }); emit(); return Promise.resolve(snap()); },
    RecoverDraft: () => Promise.resolve(snap()),
    DiscardDraft: () => Promise.resolve(),
    Export: () => { S.export_dir = set.output_dir + "\\" + S.name; emit(); return Promise.resolve({ dir: S.export_dir, state: snap() }); },
    OpenFolder: () => { console.log("[mock] OpenFolder"); return Promise.resolve(); },
    FetchNeeded: () => Promise.resolve({ online: false, error: "mock offline", activities: [] }),
    PickOutputDir: () => Promise.resolve(""),
    SetAlwaysOnTop: (on) => { set.always_on_top = on; return Promise.resolve(); },
    BeginHotkeyCapture: () => Promise.resolve(),
    EndHotkeyCapture: () => Promise.resolve({ hotkey: set.hotkey, error: "" }),
    SetHotkey: (s) => { set.hotkey = s; return Promise.resolve({ hotkey: s, error: "" }); },
    CaptureStatus: () => Promise.resolve(S.phase === "running" ? "running" : "none"),
    OpenSendDir: () => { console.log("[mock] OpenSendDir"); return Promise.resolve(); },
    OpenCaptureDir: (s) => { console.log("[mock] OpenCaptureDir", s); return Promise.resolve(); },
    BrowseExe: () => Promise.resolve("C:\\SteamLibrary\\steamapps\\common\\New World\\Bin64\\NewWorld.exe"),
    RetryInstall: () => {
      const msgs = ["Retrying environment setup...", "Downloading Python 3.12.9...", "Extracting...", "Environment setup complete!"];
      let i = 0;
      bus.emitLocal("capstatus", "installing");
      const iv = setInterval(() => {
        bus.emitLocal("caplog", msgs[i++]);
        if (i >= msgs.length) { clearInterval(iv); bus.emitLocal("capstatus", "installed"); }
      }, 300);
    },
  };

  window.CaptureBridge = { real: false, api, bus, win, params };
})();
