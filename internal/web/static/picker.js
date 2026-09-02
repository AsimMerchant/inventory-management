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
    var except = box.getAttribute('data-except') || '';
    var newLabel = box.getAttribute('data-newlabel') || '';
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
      if (except) extra += '&except=' + encodeURIComponent(except);
      req.open('GET', endpoint + '?mode=' + mode + '&q=' + encodeURIComponent(q) + extra);
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
      if (newLabel && q.trim() !== '') list.appendChild(newRow(q.trim()));
      list.hidden = !list.firstChild;
      paint();
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
      d.textContent = '+ Add "' + q + '" ' + newLabel;
      d.addEventListener('mousedown', function (e) {
        e.preventDefault();
        var form = document.querySelector('[data-newform]');
        if (!form) return;
        form.querySelector('[data-newname]').value = q;
        form.submit();
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
      text.value = rows[i].name;
      id.value = rows[i].id;
      hide();
    }

    function hide() { list.hidden = true; rows = []; here = -1; }
  }
})();
