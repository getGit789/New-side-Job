// Confirmation for destructive forms. The server enforces every rule; this only prevents mis-clicks.
document.addEventListener("submit", function (e) {
  var msg = e.target.getAttribute("data-confirm");
  if (msg && !window.confirm(msg)) e.preventDefault();
});
// Mark the current section in the navigation.
(function () {
  var path = location.pathname;
  document.querySelectorAll(".nav a[href]").forEach(function (a) {
    var href = a.getAttribute("href");
    if (href === "/" ? path === "/" : path === href || path.indexOf(href + "/") === 0) a.setAttribute("aria-current", "page");
  });
})();
// Phone menu: starts closed, opens as a bottom sheet, closes on backdrop tap or Escape.
// On wide screens it is always open. Without this script it stays open, which still works.
(function () {
  var mq = window.matchMedia("(max-width: 880px)");
  var drawers = document.querySelectorAll("details.drawer");
  function sync() {
    var open = false;
    drawers.forEach(function (d) { if (mq.matches && d.open) open = true; });
    document.body.classList.toggle("sheet-open", open);
  }
  function apply() { drawers.forEach(function (d) { d.open = !mq.matches; }); sync(); }
  apply();
  mq.addEventListener("change", apply);
  drawers.forEach(function (d) {
    d.addEventListener("toggle", sync);
    d.addEventListener("click", function (e) { if (e.target === d && mq.matches) d.open = false; });
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && mq.matches) drawers.forEach(function (d) { d.open = false; });
  });
})();
