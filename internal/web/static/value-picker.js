// The party, purpose and payment-mode picker. Suggests what the ledger already
// knows and lets a genuinely new value be typed, which is saved with the record
// itself. The server repeats this resolution inside the write, so everything
// here is convenience only.
(function () {
  var boxes = document.querySelectorAll('[data-values]');
  for (var i = 0; i < boxes.length; i++) wire(boxes[i]);

  function wire(box) {
    var text = box.querySelector('[data-values-text]');
    var id = box.querySelector('[data-values-id]');
    var list = box.querySelector('[data-values-list]');
    var kind = box.getAttribute('data-kind');
    var rows = [];
    var here = -1;

    text.addEventListener('input', function () { id.value = ''; load(text.value); });
    text.addEventListener('focus', function () { load(text.value); });
    text.addEventListener('keydown', function (e) {
      if (!rows.length) return;
      if (e.key === 'ArrowDown') { move(1); e.preventDefault(); }
      else if (e.key === 'ArrowUp') { move(-1); e.preventDefault(); }
      else if (e.key === 'Enter') { choose(here); e.preventDefault(); }
      else if (e.key === 'Escape') { hide(); }
    });
    document.addEventListener('click', function (e) {
      if (!box.contains(e.target)) hide();
    });

    function load(q) {
      var req = new XMLHttpRequest();
      req.open('GET', '/finance/api/values?kind=' + kind + '&q=' + encodeURIComponent(q));
      req.onload = function () {
        if (req.status !== 200) { hide(); return; }
        draw(JSON.parse(req.responseText), q);
      };
      req.onerror = hide;
      req.send();
    }

    // A value typed into another row of this same form has not been saved
    // yet, so the server cannot know about it. Offer it here anyway: one
    // settlement split into four amounts often repeats the purpose, and being
    // made to type it again — differently, by accident — is how a list rots.
    function pending(q) {
      var want = q.trim().toLowerCase();
      var out = [];
      var seen = {};
      var all = document.querySelectorAll('[data-values][data-kind="' + kind + '"]');
      for (var i = 0; i < all.length; i++) {
        if (all[i] === box) continue;
        var other = all[i].querySelector('[data-values-text]');
        var otherID = all[i].querySelector('[data-values-id]');
        if (!other || !otherID || otherID.value !== '') continue;
        var v = other.value.trim();
        var key = v.toLowerCase();
        if (v === '' || seen[key]) continue;
        if (want !== '' && key.indexOf(want) !== 0) continue;
        seen[key] = true;
        out.push({ id: '', value: v, label: v });
      }
      return out;
    }

    function draw(found, q) {
      var extra = pending(q);
      for (var n = 0; n < extra.length; n++) {
        if (!exact(found, extra[n].value)) found = found.concat([extra[n]]);
      }
      rows = found;
      here = found.length ? 0 : -1;
      list.innerHTML = '';
      for (var i = 0; i < found.length; i++) list.appendChild(row(found[i], i));
      var typed = q.trim();
      if (typed !== '' && !exact(found, typed)) list.appendChild(newRow(typed));
      list.hidden = !list.firstChild;
      paint();
    }

    function exact(found, q) {
      for (var i = 0; i < found.length; i++) {
        if (found[i].value.toLowerCase() === q.toLowerCase()) return true;
      }
      return false;
    }

    function row(found, i) {
      var d = document.createElement('div');
      d.textContent = found.label;
      d.addEventListener('mousedown', function (e) { e.preventDefault(); choose(i); });
      return d;
    }

    // Choosing this row leaves the typed text in the box with no ID. The
    // server sees a name with no selection and creates it as it saves.
    function newRow(q) {
      var d = document.createElement('div');
      d.className = 'new';
      d.textContent = '+ Add “' + q + '”';
      d.addEventListener('mousedown', function (e) {
        e.preventDefault();
        text.value = q;
        id.value = '';
        hide();
      });
      return d;
    }

    function move(step) {
      here = here + step;
      if (here < 0) here = 0;
      if (here > rows.length - 1) here = rows.length - 1;
      paint();
    }

    function paint() {
      var kids = list.children;
      for (var i = 0; i < kids.length; i++) {
        if (kids[i].className === 'new') continue;
        kids[i].className = i === here ? 'hl' : '';
      }
    }

    function choose(i) {
      if (i < 0 || i > rows.length - 1) return;
      text.value = rows[i].value;
      id.value = rows[i].id;
      // Setting .value fires nothing, and an "input" event would trip this
      // box's own handler and blank the ID again. Screens that depend on this
      // value listen for this instead.
      text.dispatchEvent(new CustomEvent('valuepicked', { bubbles: true }));
      hide();
    }

    function hide() { list.hidden = true; rows = []; here = -1; }
  }
})();
