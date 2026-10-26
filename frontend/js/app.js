(function () {
  "use strict";

  const B = window.CaptureBridge;
  const api = B.api;
  const $ = (id) => document.getElementById(id);

  let S = null;
  let activities = [];
  let settings = { hotkey: "F8", output_dir: "", exe_path: "", timeout: 600, one_per_endpoint: false };
  let hotkeyErr = "";
  let platform = "";
  let video = null; // VideoStatus from the app; null until Boot
  let ai = null; // AIStatus from the app; null until Boot
  let aiModels = []; // ModelInfo list from the last "Load list"
  let aiFormDirty = false; // the player is editing the AI form
  const aiRows = new Map();
  const aiThumbs = new Map();


  let catalog = null;
  let nameTouched = false;
  let starting = false;
  let noticeTimer = 0;
  const rows = new Map();

  // ---- formatting -------------------------------------------------------------
  const p2 = (n) => String(n).padStart(2, "0");
  const p3 = (n) => String(n).padStart(3, "0");
  function clockParts(ms) {
    const d = new Date(ms);
    return [p2(d.getHours()) + ":" + p2(d.getMinutes()) + ":" + p2(d.getSeconds()), p3(d.getMilliseconds())];
  }
  const fmtClock = (ms) => clockParts(ms).join(".");
  function fmtRel(ms) {
    const sign = ms < 0 ? "-" : "+";
    ms = Math.abs(ms);
    const h = Math.floor(ms / 3600000), m = Math.floor(ms / 60000) % 60, s = Math.floor(ms / 1000) % 60;
    return sign + (h > 0 ? h + ":" : "") + p2(m) + ":" + p2(s) + "." + p3(ms % 1000);
  }
  function fmtDur(ms) {
    ms = Math.max(0, ms);
    return p2(Math.floor(ms / 3600000)) + ":" + p2(Math.floor(ms / 60000) % 60) + ":" + p2(Math.floor(ms / 1000) % 60) + "." + p3(ms % 1000);
  }
  function fmtWhen(ms) {
    const d = new Date(ms), today = new Date();
    const day = d.toDateString() === today.toDateString() ? "" : d.getFullYear() + "-" + p2(d.getMonth() + 1) + "-" + p2(d.getDate()) + " ";
    return day + fmtClock(ms).slice(0, 8);
  }

  function h(tag, props, ...kids) {
    const el = document.createElement(tag);
    Object.entries(props || {}).forEach(([k, v]) => {
      if (k === "class") el.className = v;
      else if (k === "text") el.textContent = v;
      else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
      else el.setAttribute(k, v);
    });
    kids.flat().forEach((c) => c != null && el.append(c));
    return el;
  }

  function notice(msg, bad) {
    const el = $("sb-note");
    el.textContent = msg;
    el.className = "cell grow" + (bad ? " bad" : "");
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => {
      el.textContent = "No telemetry. Uses Aeternum-World by Coldzer0.";
      el.className = "cell grow";
    }, 7000);
  }
  const fail = (e) => notice(String((e && e.message) || e), true);

  // ---- clock ------------------------------------------------------------------
  const clockEl = $("clock"), elapsedEl = $("elapsed");
  const clockMain = document.createTextNode("00:00:00"), clockMs = h("span", { class: "ms", text: ".000" });
  clockEl.replaceChildren(clockMain, clockMs);
  // The clock redraws 20 times a second, not on every frame: each redraw
  // repaints the panel, and that made the whole window feel slow.
  const CLOCK_STEP_MS = 50;
  let lastTick = 0;
  function tick() {
    const now = Date.now();
    if (now - lastTick < CLOCK_STEP_MS) { requestAnimationFrame(tick); return; }
    lastTick = now;
    const [hms, ms] = clockParts(now);
    if (clockMain.nodeValue !== hms) clockMain.nodeValue = hms;
    clockMs.textContent = "." + ms;
    if (S && S.phase === "running") elapsedEl.textContent = fmtDur(now - S.started_ms);
    requestAnimationFrame(tick);
  }

  // ---- render -----------------------------------------------------------------
  function render(st) {
    S = st;
    const running = S.phase === "running", stopped = S.phase === "stopped", idle = S.phase === "idle";

    $("phase").textContent = running ? "● Recording" : stopped ? "Stopped" : starting ? "Setting up…" : "Not recording";
    $("phase").className = running ? "rec" : "";
    if (stopped) elapsedEl.textContent = fmtDur(S.stopped_ms - S.started_ms);
    else if (idle && !starting) elapsedEl.textContent = "--:--:--.---";

    $("btn-mark").disabled = !running;
    $("btn-start").hidden = running || starting;
    $("btn-open-session").hidden = running || starting;
    $("btn-stop").hidden = !running && !starting;
    $("setup").hidden = stopped;
    $("done").hidden = !stopped;
    $("btn-export").disabled = idle;

    if (running) {
      $("recover").hidden = true;
    }
    if (stopped) {
      $("done-where").textContent = S.export_dir || "Not exported yet. Press Export again.";
    }

    setFieldsLocked(running || starting);
    renderSteps();
    paintVideo();
    paintAI();
    renderCards();
    renderPackets();

    const dr = $("sb-draft");
    if (S.draft_error) { dr.textContent = S.draft_error; dr.className = "cell bad"; }
    else if (S.saved_ms) { dr.textContent = "Draft saved " + fmtClock(S.saved_ms).slice(0, 8); dr.className = "cell ok"; }
    else { dr.textContent = "No draft yet"; dr.className = "cell"; }
  }

  function setFieldsLocked(locked) {
    $("cap-exe").disabled = locked;
    $("btn-browse-exe").disabled = locked;
    $("in-name").disabled = locked;
    $("in-activity").disabled = locked;
    $("cap-timeout").disabled = locked;
    $("cap-oneperep").disabled = locked;
  }

  // ---- steps ------------------------------------------------------------------
  function renderSteps() {
    const box = $("steps");
    const steps = S.steps;
    $("steps-count").textContent = steps.length ? steps.length + (steps.length === 1 ? " step" : " steps") : "";
    if (!steps.length) {
      rows.clear();
      box.replaceChildren(h("div", { class: "empty", text: S.phase === "running"
        ? "Nothing yet. Press Mark (or the hotkey) the moment you do something in the game."
        : "Start a session, then mark each thing you do." }));
      return;
    }
    const bulk = rows.size === 0;
    if (box.querySelector(".empty")) box.replaceChildren();
    const live = new Set(steps.map((s) => s.id));
    for (const [id, el] of rows) if (!live.has(id)) { el.remove(); rows.delete(id); }
    let added = false;
    steps.forEach((s, i) => {
      let el = rows.get(s.id);
      if (!el) {
        el = makeRow(s);
        rows.set(s.id, el);
        if (!bulk) { el.classList.add("new"); added = true; }
      }
      el.dataset.via = s.via;
      el.querySelector(".n").textContent = i + 1;
      el.querySelector(".t").textContent = fmtClock(s.t_ms);
      el.querySelector(".r").textContent = fmtRel(s.t_ms - S.started_ms);
      el.title = s.via + (s.target != null ? " · packet " + s.target : "");
      const inp = el.querySelector("input");
      if (document.activeElement !== inp) inp.value = s.text;
      if (box.children[i] !== el) box.insertBefore(el, box.children[i] || null);
    });
    if (added) box.scrollTop = box.scrollHeight;
  }

  function makeRow(s) {
    const inp = h("input", { type: "text", maxlength: "500", spellcheck: "false", autocomplete: "off", "aria-label": "Step text" });
    inp.addEventListener("keydown", (e) => {
      if (e.key === "Enter") inp.blur();
      if (e.key === "Escape") { inp.value = stepText(s.id); inp.blur(); }
    });
    inp.addEventListener("change", () => api.EditStep(s.id, inp.value).catch(fail));
    return h("div", { class: "step", role: "listitem" },
      h("span", { class: "n" }), h("span", { class: "t" }), h("span", { class: "r" }), inp,
      h("button", { class: "del", type: "button", title: "Delete this step", "aria-label": "Delete step",
        onclick: () => api.DeleteStep(s.id).catch(fail) }, "×"));
  }
  const stepText = (id) => (S.steps.find((x) => x.id === id) || {}).text || "";

  // ---- unified session actions ------------------------------------------------
  async function suggest() {
    if (nameTouched) return;
    $("in-name").value = await api.SuggestName($("in-activity").value);
  }

  async function doStart() {
    $("session-err").hidden = true;
    const exe = $("cap-exe").value.trim();
    if (!exe) { notice("Set the executable path first.", true); $("cap-exe").focus(); return; }

    let name = $("in-name").value.trim();
    if (!name) name = await api.SuggestName($("in-activity").value);
    const activity = $("in-activity").value;
    const timeout = parseInt($("cap-timeout").value) || 600;
    const onePerEP = $("cap-oneperep").checked;

    starting = true;
    setFieldsLocked(true);
    $("btn-start").hidden = true;
    $("btn-stop").hidden = false;
    $("phase").textContent = "Setting up…";

    try {
      rows.clear();
      const st = await api.StartSession(exe, name, activity, timeout, onePerEP);
      starting = false;
      render(st);
      notice("Session started — capture + journal running.");
    } catch (e) {
      starting = false;
      $("session-err").textContent = String((e && e.message) || e);
      $("session-err").hidden = false;
      $("btn-start").hidden = false;
      $("btn-stop").hidden = true;
      setFieldsLocked(false);
      fail(e);
    }
  }

  async function doStop() {
    try {
      const res = await api.StopSession();
      $("done-err").hidden = !res.export_error;
      $("done-err").textContent = res.export_error ? "Export failed: " + res.export_error : "";

      const sizeWarn = $("size-warn");
      if (res.size_warning) { sizeWarn.textContent = res.size_warning; sizeWarn.hidden = false; }
      else { sizeWarn.hidden = true; }

      render(res.state);

      if (res.send_dir) $("done-where").textContent = "Files to send: " + res.send_dir;

      const videoNote = res.video_error ? " Video: " + res.video_error
        : res.video_clips ? ` ${res.video_clips} video clip(s) kept in the session folder (not in send).` : "";
      notice((res.export_error ? "Export failed: " + res.export_error : "Saved — open the send folder to share.") + videoNote,
        !!(res.export_error || res.video_error));
    } catch (e) { fail(e); }
  }

  async function doNew() {
    rows.clear();
    nameTouched = false;
    $("split-result").hidden = true;
    $("btn-parts").hidden = true;
    paintUpload(null);
    render(await api.NewSession());
    suggest();
  }

  async function doExport() {
    try {
      const res = await api.Export();
      $("done-err").hidden = true;
      render(res.state);
      notice("Saved to " + res.dir);
    } catch (e) {
      $("done-err").textContent = "Export failed: " + e;
      $("done-err").hidden = false;
      fail(e);
    }
  }

  async function doMark() {
    try {
      const st = await api.MarkButton();
      render(st);
      flash();
      const last = st.steps.reduce((a, b) => (b.id > a.id ? b : a), st.steps[0]);
      const inp = (rows.get(last.id) || document).querySelector("input");
      if (inp) { inp.focus(); inp.select(); }
    } catch (e) { fail(e); }
  }

  function flash() {
    const b = $("btn-mark");
    b.classList.remove("flash");
    void b.offsetWidth;
    b.classList.add("flash");
  }

  // ---- log --------------------------------------------------------------------
  function appendLog(text) {
    const box = $("logbox");
    const empty = box.querySelector(".empty");
    if (empty) empty.remove();
    const line = document.createElement("div");
    line.textContent = text;
    box.appendChild(line);
    box.scrollTop = box.scrollHeight;
  }

  function updateCapStatus(st) {
    const el = $("sb-cap");
    const retry = $("retry-row");
    switch (st) {
      case "installing":
        el.textContent = "Setting up…"; el.className = "cell warn";
        if (retry) retry.hidden = true;
        break;
      case "installed":
        el.textContent = "Capture: ready"; el.className = "cell ok";
        if (retry) retry.hidden = true;
        break;
      case "running": el.textContent = "Capture: running"; el.className = "cell rec"; break;
      case "done": el.textContent = "Capture: done"; el.className = "cell ok"; break;
      case "error":
        el.textContent = "Capture: error"; el.className = "cell bad";
        if (retry) retry.hidden = false;
        break;
      default: el.textContent = "Capture: idle"; el.className = "cell";
    }
  }

  function doOpenSend() {
    api.OpenSendDir().catch(fail);
  }

  function doBrowseCap() {
    const name = $("in-name").value.trim() || "my_session";
    api.OpenCaptureDir(name).catch(fail);
  }

  async function doBrowseExe() {
    try {
      const path = await api.BrowseExe();
      if (path) $("cap-exe").value = path;
    } catch (e) { fail(e); }
  }

  function doRetryInstall() {
    $("retry-row").hidden = true;
    api.RetryInstall();
  }

  // ---- missions ---------------------------------------------------------------
  async function loadNeeded(manual) {
    const st = $("m-status");
    st.className = "mstatus";
    st.textContent = "Loading the list…";
    $("btn-load").disabled = true;
    try {
      catalog = await api.FetchNeeded();
    } catch (e) {
      catalog = { online: false, error: String(e), activities: [] };
    }
    $("btn-load").disabled = false;
    const sel = $("m-activity");
    sel.replaceChildren();
    if (!catalog.online || !catalog.activities.length) {
      sel.disabled = true;
      $("pklist").hidden = true;
      st.className = "mstatus off";
      st.textContent = catalog.online
        ? "The list is empty right now. Everything else still works."
        : "Offline: missions are not available. Everything else works without them.";
      if (!catalog.online && manual) notice("Could not load missions: " + catalog.error, true);
      renderCards();
      return;
    }
    catalog.activities.forEach((a) => {
      const wanted = a.packets.filter((p) => p.status === "wanted").length;
      sel.append(h("option", { value: a.activity, text: a.activity + " (" + wanted + " wanted)" }));
    });
    sel.disabled = false;
    $("btn-load").textContent = "Refresh";
    st.textContent = "Tick the packets you want to record. Each one shows a how-to you can click through.";
    renderPackets();
    renderCards();
  }

  const packetIndex = () => {
    const m = new Map();
    if (catalog) catalog.activities.forEach((a) => a.packets.forEach((p) => m.set(p.idx, p)));
    return m;
  };

  function renderPackets() {
    const box = $("pklist");
    if (!catalog || !catalog.online || !catalog.activities.length) { box.hidden = true; return; }
    const act = catalog.activities.find((a) => a.activity === $("m-activity").value) || catalog.activities[0];
    box.hidden = false;
    const list = act.packets.slice().sort((a, b) => (a.status === "wanted" ? 0 : 1) - (b.status === "wanted" ? 0 : 1));
    box.replaceChildren(...list.map((p) => {
      const cb = h("input", { type: "checkbox" });
      cb.checked = S.targets.includes(p.idx);
      cb.addEventListener("change", () => {
        const next = cb.checked ? S.targets.concat(p.idx) : S.targets.filter((x) => x !== p.idx);
        api.SetTargets(next).then(render).catch(fail);
      });
      return h("label", { class: "pk", title: p.name },
        cb, h("span", { class: "nm", text: p.short || p.name }), h("span", { class: "ix", text: "#" + p.idx }),
        h("span", { class: "st " + p.status, text: p.status }));
    }));
  }

  function renderCards() {
    const box = $("cards");
    const idx = packetIndex();
    const running = S && S.phase === "running";
    box.replaceChildren(...((S && S.targets) || []).map((id) => {
      const p = idx.get(id);
      if (!p) {
        return h("div", { class: "card" }, h("h3", { text: "Packet #" + id }),
          h("p", { class: "what", text: "This packet is not in the list right now (offline or already solved)." }));
      }
      const items = p.how_to_capture.map((text, i) => {
        const n = S.steps.filter((s) => s.via === "checklist" && s.target === id && s.text === text).length;
        return h("li", {}, h("button", { class: "cl", type: "button", disabled: running ? null : "disabled",
          title: running ? "Mark this step now" : "Start a session to mark steps",
          onclick: () => api.MarkChecklist(text, id).then(render).catch(fail) },
        h("span", { class: "num", text: (i + 1) + "." }), h("span", { text }),
        n ? h("span", { class: "cnt", text: "✓ ×" + n }) : null));
      });
      return h("div", { class: "card" },
        h("h3", { text: p.short || p.name }),
        h("span", { class: "ix", text: "#" + p.idx + " · " + p.status }), h("br"),
        h("span", { class: "conf " + p.confidence, text: p.confidence_label }),
        p.what ? h("p", { class: "what", text: p.what }) : null,
        p.notes ? h("p", { class: "notes", text: p.notes }) : null,
        items.length ? h("ol", {}, items) : h("p", { class: "notes", text: "No how-to for this one yet. Any long session in a busy area helps." }),
        items.length && !running ? h("p", { class: "cardhint", text: "Start a session to click through the steps." }) : null);
    }));
  }

  // ---- hotkey -----------------------------------------------------------------
  const KEYNAMES = { Space: "Space", Insert: "Insert", Delete: "Delete", Home: "Home", End: "End", PageUp: "PageUp", PageDown: "PageDown", Pause: "Pause", ScrollLock: "ScrollLock" };
  function keyFromEvent(e) {
    const c = e.code;
    if (/^F\d{1,2}$/.test(c)) return c;
    if (/^Key[A-Z]$/.test(c)) return c.slice(3);
    if (/^Digit\d$/.test(c)) return c.slice(5);
    if (/^Numpad\d$/.test(c)) return "Num" + c.slice(6);
    return KEYNAMES[c] || "";
  }

  function paintHotkey() {
    $("hk-label").textContent = settings.hotkey;
    $("s-hotkey").textContent = settings.hotkey;
    const c = $("sb-hk");
    if (hotkeyErr) { c.textContent = "Hotkey unavailable: " + hotkeyErr; c.className = "cell bad"; }
    else { c.textContent = "Hotkey " + settings.hotkey + " ready"; c.className = "cell ok"; }
  }

  // captureHotkey lets the player press a new combination for one hotkey.
  // set sends it to the app; apply stores what the app answered. Both
  // hotkeys are paused while a combination is being pressed.
  let capturing = false;
  function captureHotkey(btnId, valId, set, apply) {
    if (capturing) return;
    capturing = true;
    const btn = $(btnId), err = btn.closest(".kv").querySelector(".err"); // the error line of this key's tab
    err.hidden = true;
    btn.textContent = "Press keys…";
    $(valId).textContent = "Press the new combination (Esc cancels)";
    api.BeginHotkeyCapture();
    const finish = async (status) => {
      window.removeEventListener("keydown", onKey, true);
      capturing = false;
      btn.textContent = "Change";
      if (status) apply(status);
      else {
        const hs = await api.EndHotkeyCapture();
        settings.hotkey = hs.hotkey; hotkeyErr = hs.error;
        video = await api.VideoState();
      }
      paintHotkey();
      paintVideo();
    };
    const onKey = async (e) => {
      if (["Control", "Alt", "Shift", "Meta"].includes(e.key)) return;
      e.preventDefault(); e.stopPropagation();
      if (e.key === "Escape") { finish(null); return; }
      const key = keyFromEvent(e);
      if (!key) { err.textContent = "That key can't be used as a hotkey."; err.hidden = false; return; }
      const combo = [e.ctrlKey && "Ctrl", e.altKey && "Alt", e.shiftKey && "Shift", e.metaKey && "Win", key].filter(Boolean).join("+");
      try {
        finish(await set(combo));
      } catch (ex) {
        err.textContent = String((ex && ex.message) || ex); err.hidden = false;
        finish(null);
      }
    };
    window.addEventListener("keydown", onKey, true);
  }

  // ---- video (opt-in; Linux and Windows) ------------------------------------------
  function paintVideo() {
    const has = !!video;
    const show = (sel, on) => document.querySelectorAll(sel).forEach((el) => { el.hidden = !on; });
    const ready = has && video.info.available;
    const allowed = ready && video.allowed;
    const win = ready && video.info.window_capture;
    show(".video-row", has);
    show(".video-opt", allowed);
    show(".video-mon", allowed && !win);
    show(".video-win", allowed && video.info.needs_pick);
    show(".video-sound", allowed && video.info.sound);
    $("btn-video").hidden = !allowed;
    $("video-hint").hidden = !allowed;
    if (!has) return;

    const note = $("s-video-note"), allowBtn = $("btn-video-allow");
    note.hidden = false;
    allowBtn.disabled = false;
    if (!ready) {
      $("s-video").textContent = "Not available";
      allowBtn.textContent = video.info.can_install ? "Install" : "Check again";
      note.textContent = video.error ? "Video: " + video.error : video.info.reason;
      return;
    }
    $("s-video").textContent = !video.allowed ? "Off" : video.recording ? "On, recording" : "On";
    allowBtn.textContent = video.allowed ? "Turn off" : "Turn on";
    const problem = video.error || video.hotkey_error;
    note.textContent = problem ? "Video: " + problem
      : !video.allowed ? "Off by default. When on, a clip starts with each session; the video key stops and starts it."
      : video.info.needs_pick && !video.window_picked ? "With New World open, press Pick window and choose the New World window. You do this once."
      : !win && !video.target ? "Choose the monitor New World is on."
      : "Clips stay in the session folder on this PC, never in send. " +
        (video.info.sound ? "Sound is the game's own sound only; a microphone is never recorded."
          : "Clips record the New World window, without sound.") +
        (video.clips ? ` ${video.clips} clip(s) this session.` : "");

    $("s-video-window").textContent = video.window_picked ? "Game window" : "Not picked yet";
    const sel = $("s-video-target"), monitors = video.info.monitors || [];
    const values = ["", ...monitors.map((m) => m.name)];
    if ([...sel.options].map((o) => o.value).join("|") !== values.join("|")) {
      sel.replaceChildren(h("option", { value: "", text: "Choose the game's monitor" }),
        ...monitors.map((m) => h("option", { value: m.name, text: `${m.name} (${m.size})` })));
    }
    sel.value = win ? "" : video.target;
    $("s-video-audio").value = video.audio;
    $("s-video-codec").value = video.codec;
    $("s-video-quality").value = video.quality;
    $("s-vhotkey").textContent = video.hotkey;
    $("vhk-label").textContent = video.hotkey;

    const b = $("btn-video"), running = S && S.phase === "running";
    b.disabled = !running && !video.recording;
    b.textContent = video.recording ? "● Stop video" : "Start video";
    b.classList.toggle("rec", video.recording);
  }

  async function doVideoAllow() {
    try {
      if (!video.info.available && video.info.can_install) {
        $("btn-video-allow").disabled = true; // until the "video" event after the download
        api.InstallVideo();
        notice("Downloading ffmpeg; see the Activity Log.");
        return;
      }
      video = video.info.available ? await api.SetVideoAllowed(!video.allowed) : await api.RefreshVideo();
      paintVideo();
    } catch (e) { fail(e); }
  }

  async function doPickWindow() {
    const b = $("btn-video-pick");
    b.disabled = true; b.textContent = "Choose in the dialog…";
    try { video = await api.PickVideoWindow(); }
    catch (e) { fail(e); }
    finally { b.disabled = false; b.textContent = "Pick window"; paintVideo(); }
  }

  async function doToggleVideo() {
    try { video = await api.ToggleVideo(); paintVideo(); } catch (e) { fail(e); }
  }

  // ---- right column tabs ------------------------------------------------------------
  // Missions, Settings and AI marks share the right column; one shows at a
  // time. The choice is remembered on this PC.
  const TAB_KEY = "azoth.rightTab";
  function showTab(id) {
    document.querySelectorAll(".tabs [data-tab]").forEach((b) => {
      const on = b.dataset.tab === id;
      b.setAttribute("aria-selected", on);
      $(b.dataset.tab).classList.toggle("tab-off", !on);
    });
    try { localStorage.setItem(TAB_KEY, id); } catch (e) { /* storage blocked: the tab still works */ }
  }
  function wireTabs() {
    document.querySelectorAll(".tabs [data-tab]").forEach((b) => b.addEventListener("click", () => showTab(b.dataset.tab)));
    let saved = "missions";
    try { saved = localStorage.getItem(TAB_KEY) || saved; } catch (e) { /* use the first tab */ }
    showTab($(saved) ? saved : "missions");
  }

  // ---- upload zip -----------------------------------------------------------------
  // paintUpload shows the zip the app made after Stop (and again after each
  // change to steps.json), with anything the upload site would reject.
  function paintUpload(up) {
    const box = $("upload");
    box.hidden = !up;
    if (!up) return;
    $("upload-path").textContent = up.error ? "Upload zip not made: " + up.error : "Upload zip: " + up.path;
    $("btn-upload-open").hidden = !!up.error;
    $("upload-problems").replaceChildren(...(up.problems || []).map((p) => h("div", { class: "bad", text: p })));
  }

  // ---- split by connection --------------------------------------------------------
  async function doSplit() {
    const box = $("split-result"), b = $("btn-split");
    b.disabled = true;
    try {
      const res = await api.SplitSession();
      box.replaceChildren(h("div", { text: res.parts.length + " parts, one per login, in " + res.dir }),
        ...res.parts.map((p) => h("div", { class: p.problem ? "bad" : "" ,
          text: "Part " + p.number + ": " + fmtClock(p.first_ms).slice(0, 8) + "–" + fmtClock(p.last_ms).slice(0, 8) +
            ", " + p.steps + " step(s), " + (p.bytes / 1048576).toFixed(1) + " MB" + (p.problem ? " – " + p.problem : "") })));
      $("btn-parts").hidden = false;
    } catch (e) {
      box.replaceChildren(h("div", { text: String((e && e.message) || e) }));
      $("btn-parts").hidden = true;
    } finally {
      box.hidden = false;
      b.disabled = false;
    }
  }

  // ---- AI marks (opt-in, needs video) -------------------------------------------
  const aiShow = (sel, on) => document.querySelectorAll(sel).forEach((el) => { el.hidden = !on; });

  function aiForm() {
    return Object.assign({}, ai.settings, {
      connector: $("ai-connector").value,
      cli: $("ai-cli").value,
      base_url: $("ai-url").value.trim(),
      api_key: $("ai-key").value.trim(),
      model: $("ai-model-in").value.trim(),
      effort: $("ai-effort").value.trim(),
      data_source: $("ai-data").value,
      sample_every: parseInt($("ai-every").value) || 0,
    });
  }

  function fillAIForm() {
    const s = ai.settings;
    const cli = $("ai-cli");
    if (cli.options.length !== ai.clis.length) cli.replaceChildren(...ai.clis.map((n) => h("option", { value: n, text: n })));
    $("ai-connector").value = s.connector || "cli";
    cli.value = s.cli || ai.clis[0];
    $("ai-url").value = s.base_url;
    $("ai-key").value = "";
    $("ai-key").placeholder = ai.has_key ? "Saved. Type to replace it." : "Not needed for a local server";
    paintModels(s.model, s.effort);
    $("ai-data").value = s.data_source;
    $("ai-every").value = s.sample_every;
    $("ai-datadir").textContent = s.data_dir || "No folder picked";
    $("ai-datadir").title = s.data_dir || "";
  }

  function paintAIFormParts() {
    const c = $("ai-connector").value;
    aiShow(".ai-cli", c === "cli");
    aiShow(".ai-openai", c === "openai");
    aiShow(".ai-key", c !== "cli");
    aiShow(".ai-local", $("ai-data").value === "local");
  }

  function paintAI() {
    // The panel and the review never depend on the recorder being found at
    // this moment: settings and saved drafts work without it.
    const ready = !!ai && !!video;
    $("ai-panel").hidden = !ready;
    $("tab-ai").hidden = !ready;
    if (!ready && $("tab-ai").getAttribute("aria-selected") === "true") showTab("missions");
    // Every stopped session gets the box, so AI marks can run on a session
    // that never had a run (AI was off, or the session was recovered).
    const stopped = S && S.phase === "stopped";
    $("ai-review").hidden = !ready || !stopped;
    if (!ready) return;

    const s = ai.settings;
    $("btn-ai-enable").textContent = s.enabled ? "Turn off" : "Turn on";
    $("ai-note").textContent = !video.info.available ? "Video is not available right now (see the Video row), so new runs cannot start. Saved drafts can still be reviewed."
      : !video.allowed ? "Turn on video first. AI marks read the session's video."
      : s.enabled ? "On. After you stop a session, the model you picked reads the video and drafts marks for you to check."
      : "Off. When on, a model you pick drafts marks from the video after each session. You check every draft.";
    if (!aiFormDirty) fillAIForm();
    paintAIFormParts();
    $("ai-stats").replaceChildren(...(ai.stats || []).map((r) => {
      const n = r.accepted + r.rejected;
      return h("div", {}, h("span", { text: r.model }),
        h("span", { text: (n ? Math.round(100 * r.accepted / n) + "% kept" : "-") + " (" + r.accepted + " of " + n + ")" }));
    }));
    if (ai.stats && ai.stats.length) $("ai-stats").prepend(h("div", {}, h("b", { text: "Hit rate per model" })));

    $("btn-ai-run").hidden = ai.running;
    $("btn-ai-run").textContent = ai.result ? "Run again" : "Run AI marks";
    $("btn-ai-cancel").hidden = !ai.running;
    const res = ai.result;
    const drafts = (res && res.drafts) || [], notes = (res && res.notes) || []; // older results saved them as null
    $("ai-model").textContent = res ? res.model : "";
    $("ai-progress").textContent = ai.running ? ai.progress
      : res ? `${drafts.length} draft(s) from ${res.parts} part(s) of video, ${res.frames} frames.` +
        (res.failed ? ` ${res.failed} question(s) failed.` : "") + (notes.length ? " " + notes.join(" ") : "")
      : "";
    $("ai-err").hidden = !ai.error;
    $("ai-err").textContent = ai.error ? "AI marks: " + ai.error : "";
    renderDrafts(drafts);
  }

  function renderDrafts(drafts) {
    const box = $("ai-drafts");
    if (!drafts.length) {
      aiRows.clear();
      aiThumbs.clear(); // draft ids start again at 1 in the next run
      box.replaceChildren(h("div", { class: "empty", text: ai.running ? "Working…" : ai.result ? "No drafts." : "No AI marks for this session yet." }));
      $("ai-bulk").hidden = true;
      return;
    }
    if (box.querySelector(".empty")) box.replaceChildren();
    const live = new Set(drafts.map((d) => d.id));
    for (const [id, el] of aiRows) if (!live.has(id)) { el.remove(); aiRows.delete(id); }
    drafts.forEach((d, i) => {
      let el = aiRows.get(d.id);
      if (!el) { el = makeDraft(d); aiRows.set(d.id, el); }
      const pending = d.status === "pending";
      el.dataset.status = d.status;
      el.querySelectorAll(".acts button").forEach((b) => { b.disabled = !pending; });
      el.querySelector("input[type=text]").disabled = !pending;
      const pick = el.querySelector(".pick");
      pick.disabled = !pending;
      if (!pending) pick.checked = false;
      if (box.children[i] !== el) box.insertBefore(el, box.children[i] || null);
    });
    paintSelection();
  }

  // pendingPicks are the check boxes of the drafts still waiting for a verdict.
  const pendingPicks = () => [...$("ai-drafts").querySelectorAll(".pick:not(:disabled)")];

  function paintSelection() {
    const picks = pendingPicks(), n = picks.filter((p) => p.checked).length;
    $("ai-bulk").hidden = !picks.length;
    $("ai-selected-count").textContent = n + " of " + picks.length + " waiting drafts selected";
    $("ai-select-all").checked = picks.length > 0 && n === picks.length;
    $("ai-select-all").indeterminate = n > 0 && n < picks.length;
    $("btn-ai-accept-sel").disabled = $("btn-ai-reject-sel").disabled = !n;
  }

  // decideDrafts sends verdicts for many drafts in one call.
  function decideDrafts(decisions) {
    return api.DecideAIDrafts(decisions).then((st) => { ai = st; paintAI(); }).catch(fail);
  }

  function decideSelected(accept) {
    const decisions = pendingPicks().filter((p) => p.checked).map((p) => {
      const row = p.closest(".draft");
      return { id: Number(row.dataset.id), accept, text: row.querySelector("input[type=text]").value.trim() };
    });
    if (decisions.length) decideDrafts(decisions);
  }

  function makeDraft(d) {
    const img = h("img", { alt: "Frame of this draft", title: "Click to enlarge" });
    img.addEventListener("click", () => img.classList.toggle("big"));
    if (aiThumbs.has(d.id)) img.src = aiThumbs.get(d.id);
    else api.AIFrame(d.id).then((src) => { // "" once the frame was cleaned up
      if (!src) { img.classList.add("gone"); return; } // keeps its column, so the row does not shift
      aiThumbs.set(d.id, src); img.src = src;
    }).catch(fail);
    const inp = h("input", { type: "text", maxlength: "500", spellcheck: "false", autocomplete: "off", "aria-label": "Draft text" });
    inp.value = d.text;
    const decide = (accept) => decideDrafts([{ id: d.id, accept, text: inp.value.trim() }]);
    // Drafts the model is sure about start selected; unsure ones do not.
    const pick = h("input", { type: "checkbox", class: "pick", "aria-label": "Select this draft" });
    pick.checked = d.status === "pending" && !d.unsure;
    pick.addEventListener("change", paintSelection);
    const meta = h("div", { class: "meta" },
      h("span", { text: fmtRel(d.t_ms - S.started_ms) }), h("span", { text: d.kind }),
      h("span", { class: d.unsure ? "unsure" : "", text: Math.round(d.confidence * 100) + "%" + (d.unsure ? " unsure" : "") }));
    meta.title = d.candidates && d.candidates.length ? "Game data: " + d.candidates.join("; ") : "No game data match";
    return h("div", { class: "draft", role: "listitem", "data-id": String(d.id) }, pick, img, h("div", {}, meta, inp),
      h("div", { class: "acts" },
        h("button", { class: "btn small", type: "button", title: "Add as a step", onclick: () => decide(true) }, "✓"),
        h("button", { class: "btn small", type: "button", title: "Drop", onclick: () => decide(false) }, "✗")));
  }

  async function saveAI(patch, clearKey) {
    try {
      ai = await api.SetAISettings(Object.assign(aiForm(), patch || {}), !!clearKey);
      aiFormDirty = false;
      paintAI();
      return true;
    } catch (e) { fail(e); return false; }
  }

  // paintModels fills the model list from the last "Load list" (the saved
  // model stays in it even before a list is loaded) and picks model.
  function paintModels(model, effort) {
    const ids = aiModels.map((m) => m.id);
    if (model && !ids.includes(model)) ids.unshift(model);
    const sel = $("ai-model-in");
    sel.replaceChildren(ids.length ? null : h("option", { value: "", text: "Press Load list" }),
      ...ids.map((id) => h("option", { value: id, text: id })));
    sel.value = model || ids[0] || "";
    paintEfforts(effort);
  }

  // paintEfforts offers the chosen model's efforts, lowest first. Without a
  // wanted effort it picks the lowest, so a cheap setting is the default.
  function paintEfforts(effort) {
    const m = aiModels.find((x) => x.id === $("ai-model-in").value);
    const efforts = (m && m.efforts && m.efforts.slice()) || [];
    if (effort && !efforts.includes(effort)) efforts.push(effort);
    const sel = $("ai-effort");
    sel.replaceChildren(h("option", { value: "", text: "default" }), ...efforts.map((e) => h("option", { value: e, text: e })));
    sel.value = effort !== undefined && efforts.includes(effort) ? effort : efforts[0] || "";
  }

  async function doLoadModels() {
    const b = $("btn-ai-models");
    if (!(await saveAI())) return;
    b.disabled = true; b.textContent = "Loading…";
    try {
      aiModels = await api.ListAIModels();
      paintModels($("ai-model-in").value, $("ai-effort").value);
      aiFormDirty = true; // the shown model is not saved until Save
      notice(aiModels.length + " model(s) found. Pick one, then Save.");
    } catch (e) { fail(e); }
    finally { b.disabled = false; b.textContent = "Load list"; }
  }

  function wireAI() {
    ["ai-connector", "ai-cli", "ai-url", "ai-key", "ai-model-in", "ai-effort", "ai-data", "ai-every"].forEach((id) =>
      ["input", "change"].forEach((ev) => $(id).addEventListener(ev, () => { aiFormDirty = true; paintAIFormParts(); })));
    $("ai-model-in").addEventListener("change", () => paintEfforts());
    $("ai-connector").addEventListener("change", () => { aiModels = []; paintModels("", ""); }); // another provider's list
    $("ai-cli").addEventListener("change", () => { aiModels = []; paintModels("", ""); });
    $("btn-ai-save").addEventListener("click", async () => { if (await saveAI()) notice("AI settings saved."); });
    $("btn-ai-enable").addEventListener("click", () => saveAI({ enabled: !ai.settings.enabled }));
    $("btn-ai-forget").addEventListener("click", async () => { if (await saveAI({ api_key: "" }, true)) notice("API key forgotten."); });
    $("btn-ai-models").addEventListener("click", doLoadModels);
    $("btn-ai-datadir").addEventListener("click", () => api.PickAIDataDir().then((st) => { ai = st; aiFormDirty = false; paintAI(); }).catch(fail));
    $("btn-ai-run").addEventListener("click", () => api.RunAIMarks().then((st) => { ai = st; paintAI(); }).catch(fail));
    $("ai-select-all").addEventListener("change", (e) => {
      pendingPicks().forEach((p) => { p.checked = e.target.checked; });
      paintSelection();
    });
    $("btn-ai-accept-sel").addEventListener("click", () => decideSelected(true));
    $("btn-ai-reject-sel").addEventListener("click", () => decideSelected(false));
    $("btn-ai-cancel").addEventListener("click", () => api.CancelAIMarks().then((st) => { ai = st; paintAI(); }).catch(fail));
    B.bus.on("ai", (st) => { ai = st; paintAI(); });
  }

  // ---- wire -------------------------------------------------------------------
  function wire() {
    $("btn-close").addEventListener("click", () => B.win.quit());
    $("btn-min").addEventListener("click", () => B.win.minimise());
    $("btn-top").addEventListener("click", () => { const on = $("btn-top").getAttribute("aria-pressed") !== "true"; $("btn-top").setAttribute("aria-pressed", on); api.SetAlwaysOnTop(on).catch(fail); });

    $("btn-start").addEventListener("click", doStart);
    $("btn-stop").addEventListener("click", doStop);
    $("btn-mark").addEventListener("click", doMark);
    $("in-name").addEventListener("input", () => { nameTouched = true; });
    $("in-name").addEventListener("keydown", (e) => { if (e.key === "Enter") doStart(); });
    $("in-activity").addEventListener("change", suggest);
    $("btn-browse-exe").addEventListener("click", doBrowseExe);
    $("btn-retry").addEventListener("click", doRetryInstall);

    $("btn-export").addEventListener("click", doExport);
    $("btn-export2").addEventListener("click", doExport);
    $("btn-new").addEventListener("click", doNew);
    $("btn-send").addEventListener("click", doOpenSend);
    $("btn-open").addEventListener("click", () => api.OpenFolder().catch(fail));
    $("btn-folder").addEventListener("click", () => api.OpenFolder().catch(fail));
    $("btn-browse-cap").addEventListener("click", doBrowseCap);
    $("btn-split").addEventListener("click", doSplit);
    $("btn-open-session").addEventListener("click", async () => {
      try {
        rows.clear();
        render(await api.OpenSession());
        $("done-where").textContent = S.export_dir;
      } catch (e) { fail(e); }
    });
    $("btn-upload-open").addEventListener("click", () => api.OpenUploadZip().catch(fail));
    $("btn-upload-page").addEventListener("click", () => api.OpenUploadPage());
    B.bus.on("upload", (up) => paintUpload(up));
    $("btn-parts").addEventListener("click", () => api.OpenPartsDir().catch(fail));

    $("btn-log-copy").addEventListener("click", () => {
      navigator.clipboard.writeText($("logbox").textContent).then(() => notice("Log copied to clipboard")).catch(fail);
    });
    $("btn-log-clear").addEventListener("click", () => {
      $("logbox").replaceChildren(h("div", { class: "empty", text: "Log cleared." }));
    });

    $("btn-load").addEventListener("click", () => loadNeeded(true));
    $("m-activity").addEventListener("change", () => {
      renderPackets();
      const a = $("m-activity").value;
      if (S.phase !== "running" && activities.includes(a)) { $("in-activity").value = a; suggest(); }
    });

    $("btn-hk").addEventListener("click", () => captureHotkey("btn-hk", "s-hotkey", api.SetHotkey,
      (st) => { settings.hotkey = st.hotkey; hotkeyErr = st.error; }));
    $("btn-vhk").addEventListener("click", () => captureHotkey("btn-vhk", "s-vhotkey", api.SetVideoHotkey,
      (st) => { video = st; }));
    $("btn-video").addEventListener("click", doToggleVideo);
    $("btn-video-allow").addEventListener("click", doVideoAllow);
    $("btn-video-pick").addEventListener("click", doPickWindow);
    $("s-video-target").addEventListener("change", (e) => e.target.value && api.SetVideoTarget(e.target.value).then((st) => { video = st; paintVideo(); }).catch(fail));
    $("s-video-audio").addEventListener("change", (e) => api.SetVideoAudio(e.target.value).then((st) => { video = st; paintVideo(); }).catch(fail));
    const setFormat = () => api.SetVideoFormat($("s-video-codec").value, $("s-video-quality").value)
      .then((st) => { video = st; paintVideo(); notice("Video format saved; it applies to the next clip."); }).catch(fail);
    $("s-video-codec").addEventListener("change", setFormat);
    $("s-video-quality").addEventListener("change", setFormat);
    $("btn-dir").addEventListener("click", async () => {
      try {
        const dir = await api.PickOutputDir();
        if (dir) { settings.output_dir = dir; paintDir(); suggest(); }
      } catch (e) { fail(e); }
    });

    $("btn-recover").addEventListener("click", async () => {
      try {
        rows.clear();
        const st = await api.RecoverDraft();
        $("recover").hidden = true;
        render(st);
        if (ai && st.phase === "stopped" && st.export_dir) { ai = await api.LoadAIMarks(); paintAI(); }
      } catch (e) { fail(e); }
    });
    $("btn-discard").addEventListener("click", async () => { await api.DiscardDraft(); $("recover").hidden = true; });

    B.bus.on("state", (st) => { if (st) render(st); });
    B.bus.on("marked", () => flash());
    B.bus.on("notice", (m) => notice(m));
    B.bus.on("caplog", (line) => appendLog(line));
    B.bus.on("capstatus", (st) => updateCapStatus(st));
    B.bus.on("video", (st) => { video = st; paintVideo(); paintAI(); });
    wireAI();
    wireTabs();
  }

  function paintDir() {
    $("s-dir").textContent = settings.output_dir;
    $("s-dir").title = settings.output_dir;
  }

  // Linux attaches to the game you started and only stops recording. Windows
  // does the same when the game already runs; otherwise it starts the game and
  // closes it at the timeout. An attached capture writes only the ledger, so
  // the HTTPS "one per endpoint" option does not apply on Linux.
  function applyPlatform(p) {
    platform = p;
    const linux = platform === "linux";
    $("cap-oneperep").closest("label").hidden = linux;
    $("cap-timeout").title = linux
      ? "Seconds after which recording stops. The game keeps running."
      : "Seconds after which the capture stops. A game the app started is closed; a game you started keeps running.";
  }

  async function init() {
    wire();
    requestAnimationFrame(tick);
    const boot = await api.Boot();
    $("ver").textContent = boot.version;
    activities = boot.activities;
    settings = boot.settings;
    video = boot.video || null;
    ai = boot.ai || null;
    platform = boot.platform || "";
    hotkeyErr = boot.hotkey_error || "";
    $("in-activity").replaceChildren(...activities.map((a) => h("option", { value: a, text: a })));
    paintHotkey();
    paintDir();
    render(boot.state);
    if (boot.state.phase === "idle") suggest();

    if (settings.exe_path) $("cap-exe").value = settings.exe_path;
    if (settings.timeout) $("cap-timeout").value = settings.timeout;
    if (settings.one_per_endpoint) $("cap-oneperep").checked = true;
    applyPlatform(boot.platform);
    paintVideo();
    paintAI();

    if (boot.env_ready) {
      updateCapStatus("installed");
    } else {
      updateCapStatus("installing");
    }

    if (boot.draft && boot.draft.found) {
      const d = boot.draft;
      $("recover-text").textContent = "Found an unfinished session “" + d.name + "” (" + d.activity + ", " + d.steps +
        (d.steps === 1 ? " step" : " steps") + ", last saved " + fmtWhen(d.saved_ms) + "). Recover it?";
      $("recover").hidden = false;
    }

    loadNeeded(false);
  }

  init().catch((e) => { console.error(e); notice("Startup failed: " + e, true); });
})();
