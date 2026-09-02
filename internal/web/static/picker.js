// The product picker. One product name, chosen from the list the register
// already holds; a typed name that was never picked never becomes a record.
// No framework, no network beyond this program's own /api/products.
(function () {
  var boxes = document.querySelectorAll('[data-picker]');
  for (var i = 0; i < boxes.length; i++) wire(boxes[i]);

  function wire(box) {
    var text = box.querySelector('[data-picker-text]');
    var id = box.querySelector('[data-picker-id]');
    var list = box.querySelector('[data-picker-list]');
    var mode = box.getAttribute('data-mode') || 'all';
    var endpoint = box.getAttribute('data-endpoint') || '/api/products';
    // Extra context some lists need, such as which supplier the goods would
    // go back to. Read fresh on every keystroke: the person may change it.
    var partySel = box.getAttribute('data-party-from');
    var countSel = box.getAttribute('data-count-into');
    var only = box.querySelector('[data-picker-only]');
    if (only) only.addEventListener('change', function () { load(text.value); });
    // What may go back depends on the supplier, so changing the supplier can
    // change the number beside a product that is already chosen. Ask the server
    // again rather than assuming: most of the time the product is still fine
    // and only the number moves. Clearing somebody's work is for the case where
    // the goods genuinely did not come from this supplier.
    if (partySel) {
      var party = document.querySelector(partySel);
      if (party) {
        // Not on every keystroke: half-typed supplier names match nothing, and
        // rechecking against one would throw the product away while the person
        // is still typing. Only a finished answer counts — pressed from the
        // list, or the field left alone.
        party.addEventListener('valuepicked', recheck);
        party.addEventListener('change', recheck);
      }
    }
    var newLabel = box.getAttribute('data-newlabel') || '';
    // The financial screens have nobody on duty, so they cannot use the desk's
    // /product/new. They post to their own route and stay on the page, because
    // a redirect would throw away a half-filled order.
    var newAt = box.getAttribute('data-new-endpoint') || '';
    var csrf = box.getAttribute('data-csrf') || '';
    var confirming = '';
    var rows = [];
    var here = -1;

    text.addEventListener('input', function () {
      id.value = '';
      load(text.value);
    });
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
      var extra = '';
      if (partySel) {
        var el = document.querySelector(partySel);
        if (el && el.value) extra += '&party=' + encodeURIComponent(el.value);
      }
      if (only && only.checked) extra += '&onlyParty=yes';
      req.open('GET', endpoint + '?mode=' + mode + '&q=' + encodeURIComponent(q) + extra);
      req.onload = function () {
        if (req.status !== 200) { hide(); return; }
        draw(JSON.parse(req.responseText), q);
      };
      req.onerror = hide;
      req.send();
    }

    // recheck re-asks with the supplier that is on screen now, because how many
    // may go back depends on who they came from. It only ever moves the number.
    // The product the person chose is theirs, and stays.
    function recheck() {
      if (!id.value || partyMissing()) return;
      var chosen = id.value;
      var req = new XMLHttpRequest();
      var el = document.querySelector(partySel);
      var extra = el && el.value ? '&party=' + encodeURIComponent(el.value) : '';
      req.open('GET', endpoint + '?mode=' + mode + '&q=' + encodeURIComponent(text.value) + extra);
      req.onload = function () {
        if (req.status !== 200) return;
        var found = JSON.parse(req.responseText);
        for (var i = 0; i < found.length; i++) {
          if (found[i].id === chosen) { setCount(found[i].onHand); return; }
        }
        setCount(0);
      };
      req.send();
    }

    function setCount(n) {
      if (!countSel) return;
      var out = document.querySelector(countSel);
      if (out) out.textContent = n;
    }

    function draw(found, q) {
      rows = found;
      here = found.length ? 0 : -1;
      list.innerHTML = '';
      for (var i = 0; i < found.length; i++) list.appendChild(row(found[i], i));
      if (newLabel && q.trim() !== '') list.appendChild(newRow(q.trim()));
      list.hidden = !list.firstChild;
      paint();
    }

    function partyMissing() {
      if (!partySel) return false;
      var el = document.querySelector(partySel);
      return !el || !el.value.trim();
    }

    function row(found, i) {
      var d = document.createElement('div');
      d.textContent = found.label;
      d.addEventListener('mousedown', function (e) { e.preventDefault(); choose(i); });
      return d;
    }

    function newRow(q) {
      var d = document.createElement('div');
      d.className = 'new';
      d.textContent = confirming === q.toLowerCase()
        ? 'Press again to add "' + q + '" as a separate product'
        : '+ Add "' + q + '" ' + newLabel;
      d.addEventListener('mousedown', function (e) {
        e.preventDefault();
        if (newAt) { create(q); return; }
        var form = document.querySelector('[data-newform]');
        if (!form) return;
        form.querySelector('[data-newname]').value = q;
        form.submit();
      });
      return d;
    }

    function create(q) {
      var body = 'name=' + encodeURIComponent(q) + '&csrf=' + encodeURIComponent(csrf);
      if (confirming === q.toLowerCase()) body += '&confirm=yes';
      var req = new XMLHttpRequest();
      req.open('POST', newAt);
      req.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
      req.onload = function () {
        var answer = {};
        try { answer = JSON.parse(req.responseText); } catch (e) { answer = {}; }
        if (answer.id) {
          confirming = '';
          text.value = answer.name;
          id.value = answer.id;
          setCount(0);
          hide();
          return;
        }
        confirming = answer.needsConfirm ? q.toLowerCase() : '';
        warn(answer.error || 'That product could not be added.', q);
      };
      req.send(body);
    }

    function warn(words, q) {
      list.innerHTML = '';
      var d = document.createElement('div');
      d.className = 'warn';
      d.textContent = words;
      list.appendChild(d);
      list.appendChild(newRow(q));
      rows = [];
      here = -1;
      list.hidden = false;
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
      text.value = rows[i].name;
      id.value = rows[i].id;
      // Some screens show the number beside the picker. The server drew it for
      // whatever was picked last, so it has to be told when that changes, or it
      // sits there reading 0 next to a product with plenty available.
      setCount(rows[i].onHand);
      hide();
    }

    function hide() { list.hidden = true; rows = []; here = -1; }
  }
})();
