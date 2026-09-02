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

    function draw(found, q) {
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
