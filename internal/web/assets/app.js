const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const state = {
  devices: [],
  discovered: [],
  info: null,
  authenticated: false,
  timer: null,
};

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({
  "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
}[c]));

const platformLabel = (p) => ({
  macos: "macOS", linux: "Linux", windows: "Windows", android: "Android",
}[p] || "Device");

function platformIcon(p) {
  const stroke = 'fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"';
  switch (p) {
    case "macos":
      return `<svg viewBox="0 0 24 24" ${stroke}><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/></svg>`;
    case "linux":
      return `<svg viewBox="0 0 24 24" ${stroke}><rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/></svg>`;
    case "windows":
      return `<svg viewBox="0 0 24 24" ${stroke}><rect x="4" y="4" width="7" height="7" rx="1"/><rect x="13" y="4" width="7" height="7" rx="1"/><rect x="4" y="13" width="7" height="7" rx="1"/><rect x="13" y="13" width="7" height="7" rx="1"/></svg>`;
    case "android":
      return `<svg viewBox="0 0 24 24" ${stroke}><path d="M7 10h10v6a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2z"/><path d="M9 6 8 3M15 6l1-3M7 10V8a5 5 0 0 1 10 0v2"/><path d="M10 14h.01M14 14h.01"/></svg>`;
    default:
      return `<svg viewBox="0 0 24 24" ${stroke}><rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8M12 17v4"/></svg>`;
  }
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    ...opts,
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
  });
  let data = {};
  try { data = await res.json(); } catch { /* empty response */ }
  if (res.status === 401) {
    showAuth();
    throw new Error("Please sign in");
  }
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function toast(msg, bad = false) {
  const el = $("#toast");
  el.textContent = msg;
  el.classList.toggle("bad", bad);
  el.hidden = false;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { el.hidden = true; }, 3200);
}

async function copy(text, okMsg) {
  try {
    await navigator.clipboard.writeText(text);
    toast(okMsg);
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand("copy"); toast(okMsg); }
    catch { toast("Could not copy", true); }
    ta.remove();
  }
}

function showAuth() {
  state.authenticated = false;
  $("#auth").hidden = false;
  $("#devices-section").hidden = true;
  $("#discovered-section").hidden = true;
  $("#add-btn").hidden = true;
  $("#logout-btn").hidden = true;
  $("#hub-status").className = "pill";
  $("#hub-status").innerHTML = "<i></i> locked";
  clearInterval(state.timer);
  setTimeout(() => $("#token")?.focus(), 60);
}

async function enter() {
  state.authenticated = true;
  $("#auth").hidden = true;
  $("#devices-section").hidden = false;
  $("#add-btn").hidden = false;
  $("#logout-btn").hidden = false;
  await load();
  if (!state.authenticated) return;
  clearInterval(state.timer);
  state.timer = setInterval(load, 15000);
}

async function load() {
  try {
    const [dev, disc] = await Promise.all([
      api("/api/devices"),
      api("/api/discovered").catch(() => ({ devices: [] })),
    ]);
    state.devices = dev.devices || [];
    state.discovered = disc.devices || [];
    renderDevices();
    renderDiscovered();
    $("#hub-status").className = "pill on";
    $("#hub-status").innerHTML = "<i></i> hub online";
  } catch (err) {
    if (state.authenticated) {
      $("#hub-status").className = "pill off";
      $("#hub-status").innerHTML = "<i></i> hub offline";
      toast(err.message, true);
    }
  }
}

function renderDevices() {
  const grid = $("#devices");
  const empty = $("#empty");
  const n = state.devices.length;
  $("#device-count").textContent = n ? `${n} device${n === 1 ? "" : "s"}` : "";
  empty.hidden = n !== 0;
  grid.innerHTML = state.devices.map(deviceCard).join("");
}

function deviceCard(d) {
  const online = !!d.online;
  const port = d.vnc_port || 0;
  const mode = d.mode || "gateway";
  const token = d.token || "";
  const canOpen = mode !== "direct";
  const usesToken = mode === "gateway" && !!token;
  const chips = [];
  chips.push(port ? `<span class="chip">VNC :${port}</span>` : `<span class="chip">browser only</span>`);
  if (d.banner) chips.push(`<span class="chip">${esc(d.banner)}</span>`);
  if (mode === "l4") chips.push(`<span class="chip">L4 · device password</span>`);
  if (mode === "direct") chips.push(`<span class="chip">catalog only</span>`);

  const actions = [];
  if (canOpen) actions.push(`<a class="btn primary" href="/vnc/${encodeURIComponent(d.id)}" target="_blank" rel="noopener">Open</a>`);
  if (port) actions.push(`<button class="btn ghost" data-copy-vnc="${esc(d.id)}">Copy VNC</button>`);
  actions.push(`<button class="btn ghost" data-probe="${esc(d.id)}">Probe</button>`);
  actions.push(`<button class="btn ghost danger" data-remove="${esc(d.id)}">Remove</button>`);

  return `
    <article class="device card">
      <div class="device-top">
        <span class="platform">${platformIcon(d.platform)}${esc(platformLabel(d.platform))}</span>
        <span class="status ${online ? "on" : "off"}"><i></i>${online ? "online" : "offline"}</span>
      </div>
      <h3>${esc(d.name)}</h3>
      <p class="addr">${esc(d.host)}:${esc(d.port)}</p>
      <div class="meta">${chips.join("")}</div>
      ${d.last_error ? `<p class="error small">${esc(d.last_error)}</p>` : ""}
      <div class="device-actions">${actions.join("")}</div>
      ${usesToken && port ? `<div class="token-row"><span>token</span><code>${esc(token)}</code><button class="icon-btn" data-copy-token="${esc(d.id)}" title="Copy token" aria-label="Copy token">⧉</button></div>` : ""}
    </article>`;
}

function renderDiscovered() {
  const section = $("#discovered-section");
  const list = $("#discovered");
  const entries = state.discovered.filter((e) => !state.devices.some((d) => d.host === e.host && Number(d.port) === Number(e.port)));
  section.hidden = entries.length === 0;
  list.innerHTML = entries.map((e, i) => `
    <div class="discovered-row">
      <div><strong>${esc(e.name || "VNC server")}</strong><span>${esc(e.host)}:${esc(e.port)}</span></div>
      <button class="btn ghost" data-add-discovered="${i}">Add</button>
    </div>`).join("");
  list._entries = entries;
}

function openAdd(prefill = {}) {
  const form = $("#add-form");
  form.reset();
  form.elements.name.value = prefill.name || "";
  form.elements.host.value = prefill.host || "";
  form.elements.port.value = prefill.port || 5900;
  form.elements.password.value = "";
  form.elements.platform.value = prefill.platform || "";
  form.elements.mode.value = prefill.mode || "gateway";
  form.elements.expose.checked = !!prefill.expose;
  $("#add-error").hidden = true;
  $("#add-dialog").showModal();
  setTimeout(() => form.elements.name.focus(), 60);
}

async function submitAdd(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const err = $("#add-error");
  err.hidden = true;
  const body = {
    name: form.elements.name.value.trim(),
    host: form.elements.host.value.trim(),
    port: Number(form.elements.port.value) || 5900,
    password: form.elements.password.value,
    platform: form.elements.platform.value,
    mode: form.elements.mode.value,
    expose: form.elements.expose.checked,
  };
  try {
    const out = await api("/api/devices", { method: "POST", body: JSON.stringify(body) });
    $("#add-dialog").close();
    toast(`${out.device.name} added`);
    await load();
  } catch (e) {
    err.textContent = e.message;
    err.hidden = false;
  }
}

async function probe(id) {
  try {
    await api(`/api/devices/${encodeURIComponent(id)}/probe`, { method: "POST" });
    await load();
  } catch (e) { toast(e.message, true); }
}

async function remove(id) {
  const d = state.devices.find((x) => x.id === id);
  if (!d) return;
  if (!confirm(`Remove ${d.name}? The VNC server itself is not touched.`)) return;
  try {
    await api(`/api/devices/${encodeURIComponent(id)}`, { method: "DELETE" });
    toast(`${d.name} removed`);
    await load();
  } catch (e) { toast(e.message, true); }
}

function copyVNC(id) {
  const d = state.devices.find((x) => x.id === id);
  if (!d || !d.vnc_port) return;
  const host = location.hostname || "127.0.0.1";
  const url = `vnc://${host}:${d.vnc_port}`;
  if (d.mode === "l4") {
    copy(url, "VNC address copied — use the device's VNC password");
    return;
  }
  copy(`${url}\npassword: ${d.token}`, "VNC address and token copied");
}

function copyToken(id) {
  const d = state.devices.find((x) => x.id === id);
  if (d) copy(d.token, "Token copied");
}

async function login(event) {
  event.preventDefault();
  const err = $("#login-error");
  err.hidden = true;
  try {
    await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token: $("#token").value.trim() }),
    }).then(async (r) => {
      const data = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(data.error || "Invalid token");
    });
    $("#token").value = "";
    const next = new URLSearchParams(location.search).get("next");
    await enter();
    if (next && next.startsWith("/") && !next.startsWith("//")) location.href = next;
  } catch (e) {
    err.textContent = e.message;
    err.hidden = false;
  }
}

async function logout() {
  await fetch("/api/logout", { method: "POST" }).catch(() => {});
  showAuth();
}

function bind() {
  $("#login-form").addEventListener("submit", login);
  $("#add-form").addEventListener("submit", submitAdd);
  $("#add-btn").addEventListener("click", () => openAdd());
  $("#logout-btn").addEventListener("click", logout);

  document.addEventListener("click", (e) => {
    const add = e.target.closest("[data-action='add']");
    if (add) { openAdd(); return; }
    const cp = e.target.closest("[data-copy-vnc]");
    if (cp) { copyVNC(cp.dataset.copyVnc); return; }
    const ct = e.target.closest("[data-copy-token]");
    if (ct) { copyToken(ct.dataset.copyToken); return; }
    const pr = e.target.closest("[data-probe]");
    if (pr) { probe(pr.dataset.probe); return; }
    const rm = e.target.closest("[data-remove]");
    if (rm) { remove(rm.dataset.remove); return; }
    const ad = e.target.closest("[data-add-discovered]");
    if (ad) {
      const entry = ($("#discovered")._entries || [])[Number(ad.dataset.addDiscovered)];
      if (entry) openAdd({ name: entry.name, host: entry.host, port: entry.port });
      return;
    }
    if (e.target.closest("[data-close]")) {
      $("#add-dialog").close();
    }
  });

  $("#add-dialog").addEventListener("click", (e) => {
    if (e.target === $("#add-dialog")) $("#add-dialog").close();
  });

  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && state.authenticated) load();
  });
}

(async function boot() {
  bind();
  try {
    const me = await api("/api/session");
    state.authenticated = !!me.authenticated;
  } catch { /* locked */ }
  if (state.authenticated) {
    await enter();
  } else {
    showAuth();
  }
  api("/api/info").then((info) => {
    state.info = info;
    $("#version").textContent = info.version ? `v${info.version}` : "";
  }).catch(() => {});
})();
