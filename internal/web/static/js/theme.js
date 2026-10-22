// SPDX-License-Identifier: AGPL-3.0-only
(function () {
  function systemTheme() {
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }
  try {
    var stored = window.localStorage.getItem("chastedesk-theme");
    var theme = stored === "light" || stored === "dark" ? stored : systemTheme();
    document.documentElement.setAttribute("data-theme", theme);
    document.documentElement.style.colorScheme = theme;
  } catch (error) {
    document.documentElement.setAttribute("data-theme", systemTheme());
  }
})();
