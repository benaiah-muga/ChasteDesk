// SPDX-License-Identifier: AGPL-3.0-only
(function () {
  "use strict";

  var doc = document;
  var root = doc.documentElement;

  function $(selector, scope) {
    return (scope || doc).querySelector(selector);
  }

  function $$(selector, scope) {
    return Array.prototype.slice.call((scope || doc).querySelectorAll(selector));
  }

  function isTyping(target) {
    if (!target) return false;
    var tag = target.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target.isContentEditable;
  }

  function setTheme(theme) {
    if (theme === "dark" || theme === "light") {
      root.setAttribute("data-theme", theme);
    } else {
      root.removeAttribute("data-theme");
    }
    try {
      if (theme === "dark" || theme === "light") {
        window.localStorage.setItem("chastedesk-theme", theme);
      } else {
        window.localStorage.removeItem("chastedesk-theme");
      }
    } catch (error) {
      /* storage is optional */
    }
    updateThemeControls();
  }

  function prefersDark() {
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  function effectiveTheme() {
    return root.getAttribute("data-theme") || (prefersDark() ? "dark" : "light");
  }

  function updateThemeControls() {
    var dark = effectiveTheme() === "dark";
    $$("[data-theme-toggle]").forEach(function (button) {
      button.setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
      button.setAttribute("title", dark ? "Light theme" : "Dark theme");
      button.setAttribute("aria-pressed", dark ? "true" : "false");
    });
  }

  updateThemeControls();

  doc.addEventListener("click", function (event) {
    var toggle = event.target.closest("[data-theme-toggle]");
    if (!toggle) return;
    event.preventDefault();
    setTheme(effectiveTheme() === "dark" ? "light" : "dark");
  });

  var sidebar = $("[data-sidebar]");
  var backdrop = $("[data-sidebar-backdrop]");
  var menuButton = $("[data-sidebar-toggle]");
  var sidebarClose = $("[data-sidebar-close]");

  function setSidebar(open) {
    if (!sidebar) return;
    sidebar.setAttribute("data-open", open ? "true" : "false");
    if (backdrop) backdrop.setAttribute("data-open", open ? "true" : "false");
    if (menuButton) menuButton.setAttribute("aria-expanded", open ? "true" : "false");
    doc.body.classList.toggle("nav-open", open);
    if (open) {
      var first = sidebar.querySelector("a, button");
      if (first) first.focus();
    }
  }

  if (menuButton) {
    menuButton.addEventListener("click", function () {
      setSidebar(sidebar.getAttribute("data-open") !== "true");
    });
  }
  if (sidebarClose) {
    sidebarClose.addEventListener("click", function () { setSidebar(false); });
  }
  if (backdrop) {
    backdrop.addEventListener("click", function () { setSidebar(false); });
  }

  var command = $("[data-command]");

  function commandItems() {
    if (!command) return [];
    return $$("[data-command-item]", command);
  }

  function visibleCommandItems() {
    return commandItems().filter(function (item) {
      return !item.hidden;
    });
  }

  function selectCommand(index) {
    var items = visibleCommandItems();
    if (!items.length) return;
    var clamped = Math.max(0, Math.min(index, items.length - 1));
    items.forEach(function (item, itemIndex) {
      item.setAttribute("aria-selected", itemIndex === clamped ? "true" : "false");
    });
    var target = items[clamped];
    if (target.scrollIntoView) target.scrollIntoView({ block: "nearest" });
  }

  function openCommand() {
    if (!command || typeof command.showModal !== "function" || command.open) return;
    command.showModal();
    var input = $("[data-command-input]", command);
    if (input) {
      input.value = "";
      filterCommand("");
      input.focus();
    }
    selectCommand(0);
  }

  function closeCommand() {
    if (command && command.open) command.close();
  }

  function filterCommand(query) {
    var needle = query.trim().toLowerCase();
    var items = commandItems();
    var shown = 0;
    items.forEach(function (item) {
      var haystack = (item.getAttribute("data-command-search") || item.textContent).toLowerCase();
      var match = !needle || haystack.indexOf(needle) !== -1;
      item.hidden = !match;
      if (match) shown += 1;
    });
    var empty = $("[data-command-empty]", command);
    if (empty) empty.hidden = shown !== 0;
    selectCommand(0);
  }

  if (command) {
    var commandInput = $("[data-command-input]", command);
    if (commandInput) {
      commandInput.addEventListener("input", function () { filterCommand(commandInput.value); });
      commandInput.addEventListener("keydown", function (event) {
        var items = visibleCommandItems();
        var current = items.findIndex(function (item) { return item.getAttribute("aria-selected") === "true"; });
        if (event.key === "ArrowDown") {
          event.preventDefault();
          selectCommand(current + 1);
        } else if (event.key === "ArrowUp") {
          event.preventDefault();
          selectCommand(current - 1);
        } else if (event.key === "Enter") {
          event.preventDefault();
          var chosen = items[current === -1 ? 0 : current];
          if (chosen) {
            closeCommand();
            var action = chosen.querySelector("a, button") || chosen;
            if (action.tagName === "A") window.location.href = action.getAttribute("href");
            else action.click();
          }
        }
      });
    }
    command.addEventListener("click", function (event) {
      if (event.target === command) closeCommand();
      var item = event.target.closest("[data-command-item]");
      if (item && event.target.closest("a")) closeCommand();
    });
    doc.addEventListener("keydown", function (event) {
      var meta = event.metaKey || event.ctrlKey;
      if (meta && (event.key === "k" || event.key === "K")) {
        event.preventDefault();
        openCommand();
      } else if (event.key === "Escape" && command.open) {
        closeCommand();
      }
    });
  }

  doc.addEventListener("click", function (event) {
    var trigger = event.target.closest("[data-command-open]");
    if (trigger) {
      event.preventDefault();
      openCommand();
    }
  });

  doc.addEventListener("keydown", function (event) {
    if (event.key === "Escape" && sidebar && sidebar.getAttribute("data-open") === "true") {
      setSidebar(false);
      if (menuButton) menuButton.focus();
    }
  });

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var area = doc.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      doc.body.appendChild(area);
      area.select();
      try {
        doc.execCommand("copy") ? resolve() : reject(new Error("copy failed"));
      } catch (error) {
        reject(error);
      } finally {
        doc.body.removeChild(area);
      }
    });
  }

  doc.addEventListener("click", function (event) {
    var button = event.target.closest("[data-copy]");
    if (!button) return;
    event.preventDefault();
    var target = button.getAttribute("data-copy-target");
    var value = button.getAttribute("data-copy-value") || "";
    if (target) {
      var node = doc.querySelector(target);
      if (node) value = node.textContent || "";
    }
    copyText(value).then(function () {
      var label = button.querySelector("[data-copy-label]") || button;
      var previous = label.textContent;
      button.setAttribute("data-state", "copied");
      label.textContent = "Copied";
      window.setTimeout(function () {
        button.removeAttribute("data-state");
        label.textContent = previous;
      }, 2500);
    });
  });

  doc.addEventListener("submit", function (event) {
    var form = event.target;
    var message = form.getAttribute("data-confirm");
    if (message && !window.confirm(message)) {
      event.preventDefault();
      return;
    }
    var submit = form.querySelector('[type="submit"].btn');
    if (submit) {
      submit.classList.add("is-loading");
      submit.setAttribute("aria-busy", "true");
    }
  });

  doc.addEventListener("click", function (event) {
    var dismiss = event.target.closest("[data-dismiss]");
    if (!dismiss) return;
    var closable = dismiss.closest("[data-alert]");
    if (closable) closable.remove();
  });

  $$("[data-auto-submit]").forEach(function (control) {
    control.addEventListener("change", function () {
      if (control.form) control.form.requestSubmit();
    });
  });

  $$("textarea[data-autogrow]").forEach(function (area) {
    var resize = function () {
      area.style.height = "auto";
      area.style.height = Math.min(area.scrollHeight + 2, 320) + "px";
    };
    area.addEventListener("input", resize);
    resize();
  });

  $$("[data-composer]").forEach(function (composer) {
    var input = $("[data-composer-input]", composer);
    var internalInput = $("[data-composer-internal]", composer);
    var tabs = $$("[data-composer-mode]", composer);
    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        var mode = tab.getAttribute("data-composer-mode");
        tabs.forEach(function (other) {
          var active = other === tab;
          other.classList.toggle("is-active", active);
          other.setAttribute("aria-selected", active ? "true" : "false");
        });
        if (internalInput) internalInput.value = mode === "note" ? "1" : "0";
        if (input) {
          input.placeholder = mode === "note" ? "Write an internal note. The customer never sees this." : "Write a reply to the customer…";
          input.focus({ preventScroll: true });
        }
      });
    });
    if (input) {
      input.addEventListener("keydown", function (event) {
        if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
          event.preventDefault();
          if (composer.form) composer.form.requestSubmit();
        }
      });
    }
  });

  function scrollThreads() {
    $$("[data-thread-scroll]").forEach(function (node) {
      node.scrollTop = node.scrollHeight;
    });
  }
  scrollThreads();

  doc.body.addEventListener("htmx:afterSwap", function (event) {
    scrollThreads();
    updateThemeControls();
    var target = event.detail && event.detail.target;
    if (target && target.hasAttribute && target.hasAttribute("data-thread-scroll")) {
      target.scrollTop = target.scrollHeight;
    }
  });

  doc.body.addEventListener("htmx:beforeRequest", function (event) {
    var elt = event.detail && event.detail.elt;
    if (elt && elt.classList && elt.classList.contains("btn")) {
      elt.setAttribute("aria-busy", "true");
    }
  });

  doc.body.addEventListener("htmx:afterRequest", function (event) {
    var elt = event.detail && event.detail.elt;
    if (elt && elt.classList && elt.classList.contains("btn")) {
      elt.removeAttribute("aria-busy");
    }
  });

  function connectInboxStream() {
    if (!doc.body.hasAttribute("data-inbox-stream")) return;
    if (!window.EventSource) return;
    var source = new EventSource("/api/conversations/stream");
    var pending = false;
    source.addEventListener("message", function () {
      if (pending) return;
      pending = true;
      window.setTimeout(function () {
        pending = false;
        if (window.htmx) {
          window.htmx.trigger(doc.body, "inboxRefresh");
          window.htmx.trigger(doc.body, "knowledgeRefresh");
        }
      }, 1200);
    });
    source.addEventListener("error", function () {
      /* EventSource reconnects on its own */
    });
    window.addEventListener("pagehide", function () { source.close(); });
  }

  connectInboxStream();
})();
