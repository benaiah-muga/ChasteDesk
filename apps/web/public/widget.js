// SPDX-License-Identifier: AGPL-3.0-only
(function () {
  "use strict";
  var script = document.currentScript;
  if (!script) return;
  var orgId = script.getAttribute("data-org") || "";
  var token = script.getAttribute("data-token") || "";
  var base = new URL(script.src).origin;
  if (!orgId || !token) return;

  var state = { conversationId: null, convToken: null, open: false, seen: {} };
  var panel, log, input, launcher;

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  function renderContent(el, text, citations) {
    el.textContent = text;
    if (citations && citations.length) {
      var foot = el.appendChild(document.createElement("div"));
      foot.style.fontSize = "11px";
      foot.style.opacity = "0.7";
      foot.style.marginTop = "4px";
      foot.textContent = "Sources: " + citations.map(function (c) { return (c.sourceId || "").slice(0, 8); }).join(", ");
    }
  }

  function addMessage(role, text, citations) {
    var wrap = el("div", "cd-msg cd-" + role);
    var body = el("div", "cd-body");
    renderContent(body, text, citations);
    wrap.appendChild(body);
    log.appendChild(wrap);
    log.scrollTop = log.scrollHeight;
    return wrap;
  }

  function buildPanel() {
    panel = el("div", "cd-panel");
    panel.style.cssText = "position:fixed;bottom:76px;right:20px;width:340px;max-height:520px;background:#fff;border:1px solid #e5e5e5;border-radius:12px;box-shadow:0 8px 30px rgba(0,0,0,.12);display:none;flex-direction:column;z-index:2147483000;font-family:system-ui,sans-serif;overflow:hidden;";
    var head = el("div", "cd-head");
    head.style.cssText = "background:#2563eb;color:#fff;padding:12px 16px;font-weight:600;";
    head.textContent = "Support";
    log = el("div", "cd-log");
    log.style.cssText = "flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:8px;min-height:200px;";
    var form = el("form", "cd-form");
    form.style.cssText = "display:flex;gap:6px;padding:10px;border-top:1px solid #eee;";
    input = el("input", "cd-input");
    input.style.cssText = "flex:1;border:1px solid #ddd;border-radius:8px;padding:8px;font-size:13px;";
    input.placeholder = "Type a message…";
    var send = el("button", "cd-send", "Send");
    send.type = "submit";
    send.style.cssText = "background:#2563eb;color:#fff;border:none;border-radius:8px;padding:8px 12px;cursor:pointer;";
    form.appendChild(input);
    form.appendChild(send);
    panel.appendChild(head);
    panel.appendChild(log);
    panel.appendChild(form);
    document.body.appendChild(panel);

    form.addEventListener("submit", function (e) {
      e.preventDefault();
      var text = input.value.trim();
      if (!text) return;
      input.value = "";
      ensureConversation(function () {
        addMessage("customer", text);
        fetch(base + "/api/widget/conversations/" + state.conversationId + "/messages?token=" + encodeURIComponent(token) + "&convToken=" + encodeURIComponent(state.convToken), {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ content: text }),
        }).then(function (r) {
          if (r.status === 429) addMessage("system", "You're sending messages too quickly — please wait a moment.");
        });
      });
    });

    launcher = el("button", "cd-launcher", "+");
    launcher.style.cssText = "position:fixed;bottom:20px;right:20px;width:52px;height:52px;border-radius:50%;background:#2563eb;color:#fff;border:none;font-size:22px;cursor:pointer;box-shadow:0 4px 16px rgba(0,0,0,.2);z-index:2147483000;";
    launcher.addEventListener("click", function () {
      state.open = !state.open;
      panel.style.display = state.open ? "flex" : "none";
      launcher.textContent = state.open ? "×" : "+";
      if (state.open && !state.conversationId) ensureConversation(function () { connectStream(); });
      if (state.open) connectStream();
    });
    document.body.appendChild(launcher);
  }

  function ensureConversation(cb) {
    if (state.conversationId) { cb(); return; }
    fetch(base + "/api/widget/conversations?token=" + encodeURIComponent(token), {
      method: "POST",
      headers: { "content-type": "application/json", origin: location.origin },
      body: JSON.stringify({}),
    })
      .then(function (r) { return r.json(); })
      .then(function (body) {
        state.conversationId = body.data.conversationId;
        state.convToken = body.data.token;
        connectStream();
        cb();
      });
  }

  var es;
  function connectStream() {
    if (!state.conversationId || es) return;
    es = new EventSource(base + "/api/widget/conversations/" + state.conversationId + "/stream?token=" + encodeURIComponent(token) + "&convToken=" + encodeURIComponent(state.convToken));
    es.addEventListener("message", function (e) {
      var data = JSON.parse(e.data);
      if (state.seen[data.id]) return;
      state.seen[data.id] = true;
      addMessage("agent_ai", data.content, data.citations);
    });
    es.addEventListener("action.pending", function (e) {
      var data = JSON.parse(e.data);
      if (!state.seen[data.id]) {
        state.seen[data.id] = true;
        addMessage("system", "I'm checking with a teammate on this one — you'll hear from us shortly.");
      }
    });
    es.onerror = function () {
      es.close();
      es = null;
      setTimeout(function () { if (state.open) connectStream(); }, 3000);
    };
  }

  function boot() {
    buildPanel();
    ensureConversation(function () {});
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();
