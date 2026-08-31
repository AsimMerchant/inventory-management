// The save button says what is about to happen: "Save — 500 chairs in". It is
// rendered by the server too, so a browser with no script still reads "Save".
(function () {
  var form = document.querySelector('[data-inward]');
  if (!form) return;
  var btn = form.querySelector('[data-savebtn]');
  var qty = form.querySelector('[data-qty]');
  var name = form.querySelector('[data-picker-text]');
  var id = form.querySelector('[data-picker-id]');

  function word(n) {
    for (var i = 0; i < n.length; i++) {
      var c = n.charAt(i);
      if (c >= '0' && c <= '9') return n;
      if (i > 0 && c !== c.toLowerCase() && c === c.toUpperCase()) return n;
    }
    return n.charAt(0).toLowerCase() + n.slice(1);
  }

  function paint() {
    var n = parseInt(qty.value, 10);
    if (!id.value || !name.value || !(n >= 1)) { btn.textContent = 'Save'; return; }
    btn.textContent = 'Save — ' + n + ' ' + word(name.value) + ' in';
  }

  qty.addEventListener('input', paint);
  name.addEventListener('input', paint);
  form.addEventListener('click', function () { window.setTimeout(paint, 0); });
  paint();
})();
