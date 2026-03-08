(function () {
  // Theme management
  var THEME_KEY = "bm-theme";

  function applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
  }

  function savedTheme() {
    return localStorage.getItem(THEME_KEY) || "light";
  }

  function toggleTheme() {
    var current = document.documentElement.getAttribute("data-theme") || "light";
    var next = current === "dark" ? "light" : "dark";
    localStorage.setItem(THEME_KEY, next);
    applyTheme(next);
    updateThemeButton();
  }

  function updateThemeButton() {
    var btn = document.getElementById("theme-toggle");
    if (!btn) return;
    var theme = document.documentElement.getAttribute("data-theme") || "light";
    btn.textContent = theme === "dark" ? "☀" : "☾";
    btn.title = theme === "dark" ? "Switch to light mode" : "Switch to dark mode";
  }

  // Apply saved theme immediately
  applyTheme(savedTheme());

  document.addEventListener("DOMContentLoaded", function () {
    updateThemeButton();

    var themeBtn = document.getElementById("theme-toggle");
    if (themeBtn) {
      themeBtn.addEventListener("click", toggleTheme);
    }

    // Add project form toggle
    var addBtn = document.getElementById("add-project-btn");
    var addForm = document.getElementById("add-project-form");
    if (addBtn && addForm) {
      addBtn.addEventListener("click", function () {
        addForm.classList.toggle("visible");
        if (addForm.classList.contains("visible")) {
          var inp = addForm.querySelector("input[name='name']");
          if (inp) inp.focus();
        }
      });
    }

    // Token copy button
    var copyBtn = document.getElementById("copy-token-btn");
    if (copyBtn) {
      copyBtn.addEventListener("click", function () {
        var tokenEl = document.getElementById("new-token-value");
        if (!tokenEl) return;
        var text = tokenEl.textContent;
        navigator.clipboard.writeText(text).then(function () {
          copyBtn.textContent = "Copied!";
          setTimeout(function () { copyBtn.textContent = "Copy"; }, 2000);
        }).catch(function () {
          copyBtn.textContent = "Copy failed";
          setTimeout(function () { copyBtn.textContent = "Copy"; }, 2000);
        });
      });
    }
  });
})();
