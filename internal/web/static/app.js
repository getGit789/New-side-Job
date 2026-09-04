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
// On phones the menu starts closed; on wide screens it is always open. Without this script it stays open, which still works.
(function () {
  var mq = window.matchMedia("(max-width: 880px)");
  function apply() { document.querySelectorAll("details.drawer").forEach(function (d) { d.open = !mq.matches; }); }
  apply();
  mq.addEventListener("change", apply);
})();
