// The only thing script does on the print page: open the browser's print
// dialog. Without it the page still prints from the browser's own menu.
(function () {
  var b = document.querySelector('[data-print]');
  if (b) b.addEventListener('click', function () { window.print(); });
})();
