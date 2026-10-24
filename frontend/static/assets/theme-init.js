// Apply the saved appearance before page styles and the Vue application load.
(function () {
  var preference = "system";
  try {
    var saved = localStorage.getItem("yn.appearance");
    if (saved === "light" || saved === "dark") preference = saved;
  } catch (_) {}
  var theme =
    preference === "system"
      ? matchMedia("(prefers-color-scheme: dark)").matches
        ? "dark"
        : "light"
      : preference;
  document.documentElement.dataset.theme = theme;
  document.documentElement.dataset.themePreference = preference;
  document.documentElement.style.colorScheme = theme;
  var meta = document.querySelector('meta[name="theme-color"]');
  if (!meta) {
    meta = document.createElement("meta");
    meta.name = "theme-color";
    document.head.appendChild(meta);
  }
  meta.content = theme === "dark" ? "#121713" : "#f3f5f2";
})();
