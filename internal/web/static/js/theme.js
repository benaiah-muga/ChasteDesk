// SPDX-License-Identifier: AGPL-3.0-only
(function () {
  try {
    var stored = window.localStorage.getItem("chastedesk-theme");
    if (stored === "light" || stored === "dark") {
      document.documentElement.setAttribute("data-theme", stored);
    }
  } catch (error) {
    /* storage can be unavailable in privacy modes */
  }
})();
