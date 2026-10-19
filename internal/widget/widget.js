// SPDX-License-Identifier: AGPL-3.0-only
// ChasteDesk embeddable chat widget. Reads data-token from its own script tag.
(function () {
  "use strict";

  var script = document.currentScript || (function () {
    var all = document.getElementsByTagName("script");
    return all[all.length - 1];
  })();
  var token = script && script.getAttribute("data-token");
  if (!token) return;

  var base = new URL(script.src).origin;
  var style = document.createElement("style");
  style.textContent =
    ".cd-launcher{position:fixed;right:20px;bottom:20px;width:56px;height:56px;border-radius:50%;background:#18181b;color:#fff;border:none;cursor:pointer;font-size:22px;box-shadow:0 6px 20px rgba(0,0,0,.25);z-index:2147483000}" +
    ".cd-panel{position:fixed;right:20px;bottom:88px;width:360px;max-height:70vh;display:none;flex-direction:column;background:#fff;border:1px solid #e4e4e7;border-radius:12px;box-shadow:0 12px 40px rgba(0,0,0,.18);z-index:2147483000;font:14px/1.5 system-ui,sans-serif}" +
    ".cd-panel.cd-open{display:flex}" +
    ".cd-header{padding:12px 14px;border-bottom:1px solid #e4e4e7;font-weight:600}" +
    ".cd-log{flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:8px;max-height:52vh}" +
    ".cd-msg{padding:8px 10px;border-radius:10px;max-width:85%;white-space:pre-wrap;word-wrap:break-word}" +
    ".cd-customer{align-self:flex-end;background:#18181b;color:#fff}" +
    ".cd-agent{align-self:flex-start;background:#f4f4f5;color:#18181b}" +
    ".cd-system{align-self:center;color:#71717a;font-size:12px}" +
    ".cd-form{display:flex;gap:8px;padding:10px;border-top:1px solid #e4e4e7}" +
    ".cd-input{flex:1;border:1px solid #d4d4d8;border-radius:8px;padding:8px;font:inherit}" +
    ".cd-send{border:none;background:#18181b;color:#fff;border-radius:8px;padding:8px 12px;cursor:pointer}" +
    ".cd-cite{font-size:12px;color:#52525b;margin-top:4px}";
  document.head.appendChild(style);

  var launcher = document.createElement("button");
  launcher.className = "cd-launcher";
  launcher.setAttribute("aria-label", "Open chat");
  launcher.textContent = "💬";

  var panel = document.createElement("div");
  panel.className = "cd-panel";
  panel.innerHTML = '<div class="cd-header">Support</div><div class="cd-log" id="cd-log"></div>' +
    '<form class="cd-form"><input class="cd-input" placeholder="Write a message…" autocomplete="off" /><button class="cd-send" type="submit">Send</button></form>';

  var log = panel.querySelector(".cd-log");
  var form = panel.querySelector("form");
  var input = panel.querySelector(".cd-input");
  var header = panel.querySelector(".cd-header");

  function addMessage(role, content, citations) {
    var el = document.createElement("div");
    el.className = "cd-msg cd-" + (role === "customer" ? "customer" : role === "system" ? "system" : "agent");
    el.textContent = content;
    if (citations && citations.length) {
      var cite = document.createElement("div");
      cite.className = "cd-cite";
      cite.textContent = citations
        .map(function (c, i) {
          return "[E" + (i + 1) + "] " + (c.locator || c.sourceId || "source");
        })
        .join(" · ");
      el.appendChild(cite);
    }
    log.appendChild(el);
    log.scrollTop = log.scrollHeight;
  }

  document.body.appendChild(launcher);
  document.body.appendChild(panel);
  launcher.addEventListener("click", function () {
    panel.classList.toggle("cd-open");
  });

  var conversationId = null;
  var convToken = null;
  var source = null;

  function connectStream() {
    if (source) source.close();
    var url = base + "/api/widget/conversations/" + conversationId + "/stream?token=" +
      encodeURIComponent(token) + "&convToken=" + encodeURIComponent(convToken);
    source = new EventSource(url);
    source.addEventListener("message", function (event) {
      try {
        var data = JSON.parse(event.data);
        if (data.content) addMessage(data.role || "agent_ai", data.content, data.citations);
      } catch (err) { /* ignore malformed frames */ }
    });
  }

  function send(content) {
    var headers = { "content-type": "application/json" };
    var url = base + "/api/widget/conversations";
    var init = { method: "POST", headers: headers };
    if (conversationId) {
      url += "/" + conversationId + "/messages?token=" + encodeURIComponent(token) + "&convToken=" + encodeURIComponent(convToken);
      init.body = JSON.stringify({ content: content });
    } else {
      url += "?token=" + encodeURIComponent(token);
      init.body = JSON.stringify({ firstMessage: content });
    }
    addMessage("customer", content);
    return fetch(url, init)
      .then(function (res) {
        if (!res.ok) throw new Error("send failed: " + res.status);
        return res.json();
      })
      .then(function (payload) {
        var data = payload.data || {};
        if (!conversationId && data.conversationId) {
          conversationId = data.conversationId;
          convToken = data.token;
          connectStream();
        }
      })
      .catch(function () {
        addMessage("system", "Message could not be sent. Please try again.");
      });
  }

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    var content = input.value.trim();
    if (!content) return;
    input.value = "";
    send(content);
  });

  fetch(base + "/api/widget/config?token=" + encodeURIComponent(token))
    .then(function (res) { return res.json(); })
    .then(function (payload) {
      var data = payload.data || {};
      if (data.displayName) header.textContent = data.displayName;
      if (data.theme && data.theme.primaryColor) {
        launcher.style.background = data.theme.primaryColor;
      }
    })
    .catch(function () { /* config is best-effort */ });
})();
