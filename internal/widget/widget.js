// SPDX-License-Identifier: AGPL-3.0-only
// ChasteDesk embeddable chat widget. One script tag, one shadow root, no host CSS leaks.
(function () {
  "use strict";

  var script = document.currentScript || (function () {
    var all = document.getElementsByTagName("script");
    return all[all.length - 1];
  })();
  if (!script) return;
  var token = script.getAttribute("data-token");
  if (!token) return;

  var base = new URL(script.src).origin;
  var storageKey = "chastedesk.widget." + token.slice(-32);
  var markSvg = '<svg viewBox="0 0 32 32" fill="none" aria-hidden="true"><path d="M12.5 8.5H9v15h3.5M19.5 8.5H23v15h-3.5" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"></path><circle cx="16" cy="16" r="2.6" fill="currentColor"></circle></svg>';

  var template = [
    '<style>',
    ':host{all:initial}',
    '.cd-root{position:fixed;z-index:2147483000;right:22px;bottom:22px;display:flex;flex-direction:column;align-items:flex-end;gap:12px;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:var(--cd-ink);-webkit-font-smoothing:antialiased}',
    '.cd-root.cd-left{align-items:flex-start;left:22px;right:auto}',
    '.cd-launcher{display:flex;align-items:center;gap:9px;height:54px;padding:0 18px 0 15px;border:0;border-radius:999px;background:var(--cd-accent);color:var(--cd-on-accent);font:600 14px/1 inherit;box-shadow:0 14px 34px -14px rgba(24,20,16,.55),0 2px 8px rgba(24,20,16,.16);cursor:pointer;transition:transform .18s cubic-bezier(.16,1,.3,1),box-shadow .18s cubic-bezier(.16,1,.3,1);position:relative}',
    '.cd-launcher:hover{transform:translateY(-2px);box-shadow:0 18px 40px -14px rgba(24,20,16,.6),0 3px 10px rgba(24,20,16,.18)}',
    '.cd-launcher:active{transform:translateY(0)}',
    '.cd-launcher:focus-visible,.cd-close:focus-visible,.cd-send:focus-visible,.cd-retry:focus-visible,.cd-mini:focus-visible{outline:2px solid var(--cd-ink);outline-offset:3px}',
    '.cd-launcher svg{width:22px;height:22px}',
    '.cd-launcher[aria-expanded="true"] .cd-label{display:none}',
    '.cd-unread{position:absolute;top:-3px;right:-3px;min-width:18px;height:18px;border-radius:999px;background:#b23b2e;color:#fff;font:600 10px/18px inherit;text-align:center;padding:0 4px;display:none;border:2px solid var(--cd-paper)}',
    '.cd-unread[data-show="true"]{display:block}',
    '.cd-panel{width:min(390px,calc(100vw - 32px));height:min(620px,78dvh);display:flex;flex-direction:column;background:var(--cd-paper);border:1px solid var(--cd-rule);border-radius:20px;box-shadow:0 30px 80px -30px rgba(24,20,16,.55),0 8px 24px -12px rgba(24,20,16,.25);overflow:hidden;opacity:0;transform:translateY(10px) scale(.985);pointer-events:none;transition:opacity .22s cubic-bezier(.16,1,.3,1),transform .22s cubic-bezier(.16,1,.3,1)}',
    '.cd-panel.cd-open{opacity:1;transform:none;pointer-events:auto}',
    '.cd-header{display:flex;align-items:center;gap:11px;padding:15px 16px;background:var(--cd-accent);color:var(--cd-on-accent);flex:none}',
    '.cd-header svg{width:26px;height:26px;flex:none;opacity:.95}',
    '.cd-title{min-width:0;flex:1}',
    '.cd-name{display:block;font-size:15px;font-weight:650;line-height:1.2;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.cd-status{display:block;margin-top:2px;font-size:11px;opacity:.78;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}',
    '.cd-close{display:grid;place-items:center;width:34px;height:34px;border:0;border-radius:999px;background:rgba(0,0,0,.14);color:inherit;cursor:pointer;flex:none}',
    '.cd-close:hover{background:rgba(0,0,0,.22)}',
    '.cd-close svg{width:16px;height:16px}',
    '.cd-log{flex:1;overflow-y:auto;overscroll-behavior:contain;padding:18px 16px 10px;display:flex;flex-direction:column;gap:12px;scrollbar-width:thin;scrollbar-color:var(--cd-rule) transparent}',
    '.cd-greeting{background:var(--cd-surface);border:1px solid var(--cd-rule);border-radius:4px 14px 14px 14px;padding:12px 14px;font-size:13.5px;line-height:1.55;max-width:88%;color:var(--cd-ink)}',
    '.cd-greeting strong{display:block;font-size:13px;margin-bottom:2px}',
    '.cd-greeting span{color:var(--cd-muted);font-size:12.5px}',
    '.cd-msg{max-width:86%;display:flex;flex-direction:column;gap:4px;animation:cd-in .22s cubic-bezier(.16,1,.3,1) both}',
    '.cd-msg--customer{align-self:flex-end;align-items:flex-end}',
    '.cd-msg--agent{align-self:flex-start;align-items:flex-start}',
    '.cd-bubble{padding:10px 13px;border-radius:14px;font-size:13.5px;line-height:1.55;white-space:pre-wrap;overflow-wrap:anywhere}',
    '.cd-msg--customer .cd-bubble{background:var(--cd-accent);color:var(--cd-on-accent);border-bottom-right-radius:4px}',
    '.cd-msg--agent .cd-bubble{background:var(--cd-surface);border:1px solid var(--cd-rule);color:var(--cd-ink);border-bottom-left-radius:4px}',
    '.cd-msg--human .cd-bubble{background:#eef3ef;border:1px solid #cbded3;color:#1d3a2b;border-bottom-left-radius:4px}',
    '.cd-meta{font-size:10px;color:var(--cd-faint);padding:0 3px;letter-spacing:.02em}',
    '.cd-cites{display:flex;flex-direction:column;gap:4px;margin-top:2px;width:100%}',
    '.cd-cite{display:flex;align-items:flex-start;gap:5px;font-size:10.5px;line-height:1.4;color:var(--cd-muted);background:var(--cd-surface);border:1px solid var(--cd-rule);border-radius:8px;padding:5px 7px}',
    '.cd-cite svg{width:10px;height:10px;flex:none;margin-top:1px;color:var(--cd-accent)}',
    '.cd-system{align-self:center;max-width:90%;text-align:center;font-size:11px;line-height:1.45;color:var(--cd-muted);background:rgba(0,0,0,.035);border-radius:999px;padding:5px 11px}',
    '.cd-typing{align-self:flex-start;display:none;align-items:center;gap:4px;background:var(--cd-surface);border:1px solid var(--cd-rule);border-radius:4px 14px 14px 14px;padding:11px 13px}',
    '.cd-typing[data-show="true"]{display:flex}',
    '.cd-dot{width:5px;height:5px;border-radius:999px;background:var(--cd-faint);animation:cd-bounce 1.1s infinite ease-in-out}',
    '.cd-dot:nth-child(2){animation-delay:.14s}.cd-dot:nth-child(3){animation-delay:.28s}',
    '.cd-error{margin:0 16px 8px;padding:9px 11px;border:1px solid #e6c4bd;background:#fbebe7;color:#8a2f23;border-radius:10px;font-size:11.5px;line-height:1.45;display:none;align-items:center;gap:8px}',
    '.cd-error[data-show="true"]{display:flex}',
    '.cd-error span{flex:1}',
    '.cd-retry{border:1px solid currentColor;background:transparent;color:inherit;border-radius:7px;padding:3px 8px;font:600 10.5px/1.4 inherit;cursor:pointer}',
    '.cd-composer{flex:none;padding:10px 12px 12px;border-top:1px solid var(--cd-rule);background:var(--cd-surface)}',
    '.cd-form{display:flex;align-items:flex-end;gap:7px;border:1px solid var(--cd-rule);border-radius:14px;background:var(--cd-paper);padding:6px 6px 6px 11px;transition:border-color .18s ease,box-shadow .18s ease}',
    '.cd-form:focus-within{border-color:var(--cd-accent);box-shadow:0 0 0 3px color-mix(in srgb,var(--cd-accent) 16%,transparent)}',
    '.cd-input{flex:1;min-height:28px;max-height:110px;border:0;background:transparent;color:var(--cd-ink);font:13.5px/1.45 inherit;resize:none;outline:none;padding:5px 0}',
    '.cd-input::placeholder{color:var(--cd-faint)}',
    '.cd-send{display:grid;place-items:center;width:34px;height:34px;border:0;border-radius:10px;background:var(--cd-accent);color:var(--cd-on-accent);cursor:pointer;flex:none;transition:transform .15s ease,opacity .15s ease}',
    '.cd-send:hover{transform:translateY(-1px)}',
    '.cd-send:active{transform:translateY(0)}',
    '.cd-send:disabled{opacity:.45;cursor:not-allowed;transform:none}',
    '.cd-send svg{width:16px;height:16px}',
    '.cd-hint{margin-top:6px;font-size:9.5px;color:var(--cd-faint);display:flex;justify-content:space-between;gap:8px}',
    '.cd-foot{padding:0 14px 9px;font-size:9px;letter-spacing:.03em;color:var(--cd-faint);text-align:center;background:var(--cd-surface)}',
    '.cd-foot b{font-weight:600;color:var(--cd-muted)}',
    '.cd-reconnect{position:absolute;left:50%;transform:translateX(-50%);bottom:10px;background:var(--cd-ink);color:var(--cd-paper);font-size:10px;padding:4px 9px;border-radius:999px;display:none}',
    '@keyframes cd-in{from{opacity:0;transform:translateY(5px)}to{opacity:1;transform:none}}',
    '@keyframes cd-bounce{0%,60%,100%{transform:translateY(0);opacity:.45}30%{transform:translateY(-3px);opacity:.9}}',
    '@media (max-width:520px){.cd-root{right:12px;bottom:12px;left:12px;align-items:flex-end}.cd-root.cd-left{align-items:flex-start}.cd-panel{width:100%;height:min(92dvh,calc(100dvh - 20px));border-radius:18px}.cd-launcher{height:52px;padding:0 16px 0 13px}}',
    '@media (max-width:520px){.cd-root.cd-panel-open .cd-launcher{display:none}.cd-root.cd-panel-open .cd-panel{height:calc(100dvh - 24px)}}',
    '@media (prefers-reduced-motion:reduce){*{animation:none!important;transition-duration:.01ms!important}}',
    '</style>',
    '<div class="cd-root" data-root>',
    '  <div class="cd-panel" data-panel aria-hidden="true" role="dialog" aria-label="Support chat">',
    '    <div class="cd-header">',
    '      <span data-mark>' + markSvg + '</span>',
    '      <span class="cd-title"><span class="cd-name" data-name>Support</span><span class="cd-status" data-status>Ask us anything</span></span>',
    '      <button class="cd-close" type="button" data-close aria-label="Close chat"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg></button>',
    '    </div>',
    '    <div class="cd-log" data-log role="log" aria-live="polite" aria-relevant="additions">',
    '      <div class="cd-greeting"><strong data-greeting-title>Hi there.</strong><span data-greeting-body>Ask about your account, billing, or how something works. Every answer cites its source.</span></div>',
    '    </div>',
    '    <div class="cd-typing" data-typing aria-label="The support team is typing"><span class="cd-dot"></span><span class="cd-dot"></span><span class="cd-dot"></span></div>',
    '    <div class="cd-error" data-error role="alert"><span data-error-text></span><button class="cd-retry" type="button" data-retry>Retry</button></div>',
    '    <div class="cd-composer">',
    '      <form class="cd-form" data-form>',
    '        <textarea class="cd-input" data-input rows="1" maxlength="4000" placeholder="Write a message…" aria-label="Message"></textarea>',
    '        <button class="cd-send" type="submit" data-send aria-label="Send message"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/></svg></button>',
    '      </form>',
    '      <div class="cd-hint"><span>Enter to send · Shift + Enter for a new line</span><span data-powered>Private by default</span></div>',
    '    </div>',
    '    <div class="cd-foot">Powered by <b>ChasteDesk</b> · answers cite their sources</div>',
    '    <div class="cd-reconnect" data-reconnect>Reconnecting…</div>',
    '  </div>',
    '  <button class="cd-launcher" type="button" data-launcher aria-label="Open support chat" aria-expanded="false">',
    '    <span data-launcher-mark>' + markSvg + '</span><span class="cd-label">Chat</span><span class="cd-unread" data-unread>0</span>',
    '  </button>',
    '</div>'
  ].join("");

  function start() {
    var host = document.createElement("div");
    host.setAttribute("data-chastedesk-widget", "");
    host.style.setProperty("all", "initial", "important");
    document.body.appendChild(host);
    var root = host.attachShadow ? host.attachShadow({ mode: "open" }) : host;
    root.innerHTML = template;

    var refs = {
      root: root.querySelector("[data-root]"),
      panel: root.querySelector("[data-panel]"),
      launcher: root.querySelector("[data-launcher]"),
      close: root.querySelector("[data-close]"),
      log: root.querySelector("[data-log]"),
      form: root.querySelector("[data-form]"),
      input: root.querySelector("[data-input]"),
      send: root.querySelector("[data-send]"),
      typing: root.querySelector("[data-typing]"),
      error: root.querySelector("[data-error]"),
      errorText: root.querySelector("[data-error-text]"),
      retry: root.querySelector("[data-retry]"),
      name: root.querySelector("[data-name]"),
      status: root.querySelector("[data-status]"),
      greetingTitle: root.querySelector("[data-greeting-title]"),
      greetingBody: root.querySelector("[data-greeting-body]"),
      unread: root.querySelector("[data-unread]"),
      reconnect: root.querySelector("[data-reconnect]"),
      mark: root.querySelector("[data-mark]"),
      launcherMark: root.querySelector("[data-launcher-mark]")
    };

    var state = {
      conversationId: null,
      conversationToken: null,
      stream: null,
      opened: false,
      config: null,
      sending: false,
      unread: 0,
      seen: {},
      retryText: "",
      typingTimer: null,
      resolved: false
    };

  function applyAccent(color, position) {
    var value = /^#[0-9a-fA-F]{6}$/.test(color || "") ? color : "#d96c3f";
    var r = parseInt(value.slice(1, 3), 16) / 255;
    var g = parseInt(value.slice(3, 5), 16) / 255;
    var b = parseInt(value.slice(5, 7), 16) / 255;
    var luminance = 0.2126 * r + 0.7152 * g + 0.0722 * b;
    refs.root.style.setProperty("--cd-accent", value);
    refs.root.style.setProperty("--cd-on-accent", luminance > 0.62 ? "#1d1914" : "#fffaf4");
    refs.root.style.setProperty("--cd-ink", "#241f19");
    refs.root.style.setProperty("--cd-muted", "#6b6258");
    refs.root.style.setProperty("--cd-faint", "#91877a");
    refs.root.style.setProperty("--cd-paper", "#fbf8f2");
    refs.root.style.setProperty("--cd-surface", "#fffdf8");
    refs.root.style.setProperty("--cd-rule", "#e7ded1");
    refs.root.classList.toggle("cd-left", position === "left");
  }

  applyAccent("#d96c3f", "right");

  function setExpanded(open) {
    state.opened = open;
    refs.panel.classList.toggle("cd-open", open);
    refs.panel.setAttribute("aria-hidden", open ? "false" : "true");
    refs.launcher.setAttribute("aria-expanded", open ? "true" : "false");
    refs.root.classList.toggle("cd-panel-open", open);
    if (open) {
      state.unread = 0;
      renderUnread();
      window.setTimeout(function () { refs.input.focus({ preventScroll: true }); }, 40);
      scrollLog();
    }
  }

  function renderUnread() {
    refs.unread.textContent = state.unread > 9 ? "9+" : String(state.unread);
    refs.unread.setAttribute("data-show", state.unread > 0 && !state.opened ? "true" : "false");
    refs.launcher.setAttribute("aria-label", state.unread > 0 ? "Open support chat, " + state.unread + " new messages" : "Open support chat");
  }

  function scrollLog() {
    refs.log.scrollTop = refs.log.scrollHeight;
  }

  function showError(message) {
    state.retryText = message;
    refs.errorText.textContent = message;
    refs.error.setAttribute("data-show", "true");
  }

  function clearError() {
    refs.error.setAttribute("data-show", "false");
  }

  function setTyping(show) {
    refs.typing.setAttribute("data-show", show ? "true" : "false");
    if (state.typingTimer) {
      window.clearTimeout(state.typingTimer);
      state.typingTimer = null;
    }
    if (show) {
      state.typingTimer = window.setTimeout(function () {
        refs.typing.setAttribute("data-show", "false");
      }, 90000);
      scrollLog();
    }
  }

  function timeLabel() {
    var now = new Date();
    return now.getHours().toString().padStart(2, "0") + ":" + now.getMinutes().toString().padStart(2, "0");
  }

  function addMessage(role, content, citations, id, options) {
    var key = (id || "") + "|" + role + "|" + content;
    if (state.seen[key]) return;
    state.seen[key] = true;
    var wrapper = document.createElement("div");
    var kind = role === "customer" ? "customer" : role === "agent_human" ? "human" : role === "system" ? "system" : "agent";
    if (kind === "system") {
      wrapper.className = "cd-system";
      wrapper.textContent = content;
    } else {
      wrapper.className = "cd-msg cd-msg--" + kind;
      var bubble = document.createElement("div");
      bubble.className = "cd-bubble";
      bubble.textContent = content;
      wrapper.appendChild(bubble);
      if (citations && citations.length) {
        var cites = document.createElement("div");
        cites.className = "cd-cites";
        citations.forEach(function (citation) {
          if (!citation) return;
          var chip = document.createElement("div");
          chip.className = "cd-cite";
          chip.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 21c3 0 7-1 7-8V5c0-1.25-.8-2-2-2H4c-1.25 0-2 .75-2 2v6c0 1.25.75 2 2 2h1v1c0 1-1 2-2 2"/><path d="M15 21c3 0 7-1 7-8V5c0-1.25-.8-2-2-2h-4c-1.25 0-2 .75-2 2v6c0 1.25.75 2 2 2h1v1c0 1-1 2-2 2"/></svg>';
          var text = document.createElement("span");
          var source = citation.sourceName || citation.source || citation.sourceId || "Knowledge source";
          var version = citation.version ? " · v" + citation.version : "";
          text.textContent = source + version;
          if (citation.quote) text.title = citation.quote;
          chip.appendChild(text);
          cites.appendChild(chip);
        });
        if (cites.children.length) wrapper.appendChild(cites);
      }
      var meta = document.createElement("div");
      meta.className = "cd-meta";
      var author = kind === "customer" ? "You" : kind === "human" ? "Teammate" : (state.config && state.config.displayName) || "Support";
      meta.textContent = author + " · " + timeLabel();
      wrapper.appendChild(meta);
    }
    refs.log.appendChild(wrapper);
    scrollLog();
    if (!state.opened && role !== "customer" && !(options && options.silent)) {
      state.unread += 1;
      renderUnread();
    }
  }

  function persist() {
    try {
      if (state.conversationId && state.conversationToken) {
        window.sessionStorage.setItem(storageKey, JSON.stringify({
          conversationId: state.conversationId, conversationToken: state.conversationToken
        }));
      }
    } catch (error) { /* storage is optional */ }
  }

  function restore() {
    try {
      var raw = window.sessionStorage.getItem(storageKey);
      if (!raw) return;
      var saved = JSON.parse(raw);
      if (saved && saved.conversationId && saved.conversationToken) {
        state.conversationId = saved.conversationId;
        state.conversationToken = saved.conversationToken;
        loadHistory().finally(connectStream);
      }
    } catch (error) { /* ignore malformed storage */ }
  }

  function loadHistory() {
    var url = base + "/api/widget/conversations/" + state.conversationId + "/history?token=" +
      encodeURIComponent(token) + "&convToken=" + encodeURIComponent(state.conversationToken);
    return fetch(url)
      .then(function (response) {
        if (!response.ok) throw new Error("history failed");
        return response.json();
      })
      .then(function (payload) {
        var data = payload.data || {};
        if (data.status === "resolved" || data.status === "closed") {
          state.resolved = true;
          refs.input.disabled = true;
          refs.send.disabled = true;
          refs.input.placeholder = "This conversation is resolved.";
          addMessage("system", "This conversation has been resolved. Start a new one whenever you need us.", null, null, { silent: true });
        }
        (data.messages || []).forEach(function (message) {
          addMessage(message.role, message.content, message.citations, message.id, { silent: true });
        });
      })
      .catch(function () { /* history is best effort; the live stream still works */ });
  }

  function connectStream() {
    if (state.stream) state.stream.close();
    if (!state.conversationId || !state.conversationToken) return;
    var url = base + "/api/widget/conversations/" + state.conversationId + "/stream?token=" +
      encodeURIComponent(token) + "&convToken=" + encodeURIComponent(state.conversationToken);
    state.stream = new EventSource(url);
    state.stream.addEventListener("open", function () {
      refs.reconnect.style.display = "none";
    });
    state.stream.addEventListener("message", function (event) {
      var data;
      try { data = JSON.parse(event.data); } catch (error) { return; }
      if (!data) return;
      if (data.type === "action.pending") {
        setTyping(false);
        addMessage("system", "A teammate is reviewing this action before anything changes.");
        return;
      }
      if (!data.content) return;
      setTyping(false);
      addMessage(data.role || "agent_ai", data.content, data.citations, data.id);
    });
    state.stream.addEventListener("action.pending", function () {
      addMessage("system", "A teammate is reviewing this action before anything changes.");
    });
    state.stream.addEventListener("error", function () {
      refs.reconnect.style.display = "block";
    });
  }

  function send(content) {
    if (!content || state.sending || state.resolved) return;
    clearError();
    addMessage("customer", content, null, "local-" + Date.now(), { silent: true });
    state.sending = true;
    refs.send.disabled = true;
    setTyping(true);
    var headers = { "content-type": "application/json" };
    var request;
    if (state.conversationId) {
      request = fetch(base + "/api/widget/conversations/" + state.conversationId + "/messages?token=" +
        encodeURIComponent(token) + "&convToken=" + encodeURIComponent(state.conversationToken), {
        method: "POST", headers: headers, body: JSON.stringify({ content: content })
      });
    } else {
      request = fetch(base + "/api/widget/conversations?token=" + encodeURIComponent(token), {
        method: "POST", headers: headers, body: JSON.stringify({ firstMessage: content })
      });
    }
    request
      .then(function (response) {
        if (!response.ok) throw new Error("send failed");
        return response.json();
      })
      .then(function (payload) {
        var data = payload.data || {};
        if (!state.conversationId && data.conversationId && data.token) {
          state.conversationId = data.conversationId;
          state.conversationToken = data.token;
          persist();
          connectStream();
        }
      })
      .catch(function () {
        setTyping(false);
        showError("We could not send that message. Check your connection and try again.");
        state.retryText = content;
      })
      .finally(function () {
        state.sending = false;
        refs.send.disabled = false;
        refs.input.focus({ preventScroll: true });
      });
  }

  function resizeInput() {
    refs.input.style.height = "auto";
    refs.input.style.height = Math.min(refs.input.scrollHeight, 110) + "px";
  }

  refs.launcher.addEventListener("click", function () { setExpanded(!state.opened); });
  refs.close.addEventListener("click", function () { setExpanded(false); refs.launcher.focus(); });
  refs.form.addEventListener("submit", function (event) {
    event.preventDefault();
    var content = refs.input.value.trim();
    if (!content) return;
    refs.input.value = "";
    resizeInput();
    send(content);
  });
  refs.input.addEventListener("keydown", function (event) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      refs.form.requestSubmit();
    }
  });
  refs.input.addEventListener("input", resizeInput);
  refs.retry.addEventListener("click", function () {
    if (state.retryText) {
      refs.input.value = state.retryText;
      resizeInput();
      refs.input.focus();
      clearError();
    }
  });
  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape" && state.opened) {
      setExpanded(false);
      refs.launcher.focus();
    }
  });
  refs.panel.addEventListener("keydown", function (event) {
    if (event.key !== "Tab") return;
    var focusable = refs.panel.querySelectorAll('button:not([disabled]), textarea:not([disabled])');
    if (!focusable.length) return;
    var first = focusable[0];
    var last = focusable[focusable.length - 1];
    var active = root.activeElement || document.activeElement;
    if (event.shiftKey && active === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  });

  fetch(base + "/api/widget/config?token=" + encodeURIComponent(token))
    .then(function (response) {
      if (!response.ok) throw new Error("config failed");
      return response.json();
    })
    .then(function (payload) {
      var data = payload.data || {};
      state.config = data;
      var theme = data.theme || {};
      applyAccent(theme.primaryColor, theme.position);
      if (data.displayName) {
        refs.name.textContent = data.displayName;
        refs.greetingTitle.textContent = "Hi, welcome to " + data.displayName + ".";
      }
      if (theme.greeting) {
        refs.greetingBody.textContent = theme.greeting;
      }
      refs.status.textContent = "Answers cite your help docs · humans can step in";
      return data;
    })
    .then(function () {
      restore();
    })
    .catch(function () {
      refs.status.textContent = "Widget is not enabled for this site";
      showError("This chat is not enabled for this site yet. The site owner can allow this origin in ChasteDesk settings.");
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", start);
  } else {
    start();
  }
})();
