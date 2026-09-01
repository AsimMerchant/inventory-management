// The issue button says what is about to happen - "Issue 10 chairs to Ravi" -
// and the amber line says what the taker is already holding. Both are rendered
// by the server too, so neither depends on this file.
(function () {
  var form = document.querySelector('[data-issue]');
  if (!form) return;
  var btn = form.querySelector('[data-savebtn]');
  var qty = form.querySelector('[data-qty]');
  var product = form.querySelector('[data-picker-text]');
  var id = form.querySelector('[data-picker-id]');
  var takers = form.querySelectorAll('[data-people-text]');
  var taker = takers[0];
  var mobile = form.querySelector('[data-person-mobile]');

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
    var names = [];
    for (var i = 0; i < takers.length; i++) {
      var first = takers[i].value.trim().split(/\s+/)[0];
      if (!first) { btn.textContent = 'Issue'; return; }
      names.push(first);
    }
    if (!id.value || !(n >= 1) || !names.length) { btn.textContent = 'Issue'; return; }
    var joined = names.length === 1 ? names[0] : names.slice(0, -1).join(', ') + ' and ' + names[names.length - 1];
    btn.textContent = 'Issue ' + n + ' ' + word(product.value) + ' to ' + joined;
  }

  // The amber line is drawn again whenever the person changes, by asking the
  // server for the page it would draw for these two fields. The sentence stays
  // in the template; this only moves it onto the page already open.
  function holdings() {
    var req = new XMLHttpRequest();
    req.open('GET', '/issue/new?takerName=' + encodeURIComponent(taker.value) +
             '&takerMobile=' + encodeURIComponent(mobile.value));
    req.onload = function () {
      var old = form.querySelector('[data-holding]');
      if (old) old.parentNode.removeChild(old);
      if (req.status !== 200) return;
      var drawn = new DOMParser().parseFromString(req.responseText, 'text/html');
      var line = drawn.querySelector('[data-holding]');
      if (line) form.insertBefore(line, btn.parentNode);
    };
    req.send();
  }

  qty.addEventListener('input', paint);
  taker.addEventListener('input', paint);
  taker.addEventListener('change', function () { paint(); holdings(); });
  mobile.addEventListener('change', holdings);
  for (var i = 1; i < takers.length; i++) takers[i].addEventListener('input', paint);
  paint();
})();
