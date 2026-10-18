(function () {
  "use strict";

  const B = window.CaptureBridge;
  const api = B.api;
  const $ = (id) => document.getElementById(id);

  let S = null;
  let activities = [];
  let settings = { hotkey: "F8", output_dir: "", exe_path: "", timeout: 600, one_per_endpoint: false };
  let hotkeyErr = "";
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
  function tick() {
    const now = Date.now();
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

      notice(res.export_error ? "Export failed: " + res.export_error : "Saved — open the send folder to share.", !!res.export_error);
    } catch (e) { fail(e); }
  }

  async function doNew() {
    rows.clear();
    nameTouched = false;
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

  let capturing = false;
  function captureHotkey() {
    if (capturing) return;
    capturing = true;
    const btn = $("btn-hk"), err = $("s-err");
    err.hidden = true;
    btn.textContent = "Press keys…";
    $("s-hotkey").textContent = "Press the new combination (Esc cancels)";
    api.BeginHotkeyCapture();
    const done = async (status) => {
      window.removeEventListener("keydown", onKey, true);
      capturing = false;
      btn.textContent = "Change";
      if (status) { settings.hotkey = status.hotkey; hotkeyErr = status.error; }
      paintHotkey();
    };
    const onKey = async (e) => {
      if (["Control", "Alt", "Shift", "Meta"].includes(e.key)) return;
      e.preventDefault(); e.stopPropagation();
      if (e.key === "Escape") { done(await api.EndHotkeyCapture()); return; }
      const key = keyFromEvent(e);
      if (!key) { err.textContent = "That key can't be used as a hotkey."; err.hidden = false; return; }
      const combo = [e.ctrlKey && "Ctrl", e.altKey && "Alt", e.shiftKey && "Shift", e.metaKey && "Win", key].filter(Boolean).join("+");
      try {
        done(await api.SetHotkey(combo));
      } catch (ex) {
        err.textContent = String((ex && ex.message) || ex); err.hidden = false;
        done(await api.EndHotkeyCapture());
      }
    };
    window.addEventListener("keydown", onKey, true);
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

    $("btn-hk").addEventListener("click", captureHotkey);
    $("btn-dir").addEventListener("click", async () => {
      try {
        const dir = await api.PickOutputDir();
        if (dir) { settings.output_dir = dir; paintDir(); suggest(); }
      } catch (e) { fail(e); }
    });

    $("btn-recover").addEventListener("click", async () => {
      try { rows.clear(); const st = await api.RecoverDraft(); $("recover").hidden = true; render(st); } catch (e) { fail(e); }
    });
    $("btn-discard").addEventListener("click", async () => { await api.DiscardDraft(); $("recover").hidden = true; });

    B.bus.on("state", (st) => { if (st) render(st); });
    B.bus.on("marked", () => flash());
    B.bus.on("notice", (m) => notice(m));
    B.bus.on("caplog", (line) => appendLog(line));
    B.bus.on("capstatus", (st) => updateCapStatus(st));
  }

  function paintDir() {
    $("s-dir").textContent = settings.output_dir;
    $("s-dir").title = settings.output_dir;
  }

  async function init() {
    wire();
    requestAnimationFrame(tick);
    const boot = await api.Boot();
    $("ver").textContent = boot.version;
    activities = boot.activities;
    settings = boot.settings;
    hotkeyErr = boot.hotkey_error || "";
    $("in-activity").replaceChildren(...activities.map((a) => h("option", { value: a, text: a })));
    paintHotkey();
    paintDir();
    render(boot.state);
    if (boot.state.phase === "idle") suggest();

    if (settings.exe_path) $("cap-exe").value = settings.exe_path;
    if (settings.timeout) $("cap-timeout").value = settings.timeout;
    if (settings.one_per_endpoint) $("cap-oneperep").checked = true;

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
