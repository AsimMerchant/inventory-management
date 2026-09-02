// Choosing several products for one money entry. Type to find one, press it,
// and it joins a visible list you can take things off again. Same promise as
// every other box in the register: nothing is typed from memory, and what you
// picked is on screen where you can check it.
(function () {
  var boxes = document.querySelectorAll('[data-multi]');
  for (var i = 0; i < boxes.length; i++) wire(boxes[i]);

  function wire(box) {
    var text = box.querySelector('[data-multi-text]');
    var list = box.querySelector('[data-multi-list]');
    var chosen = box.querySelector('[data-multi-chosen]');
    var field = box.getAttribute('data-field');
    var endpoint = box.getAttribute('data-endpoint') || '/finance/api/products';
    var rows = [], here = -1;

    text.addEventListener('input', function () { load(text.value); });
    text.addEventListener('focus', function () { load(text.value); });
    text.addEventListener('keydown', function (e) {
      if (!rows.length) return;
      if (e.key === 'ArrowDown') { move(1); e.preventDefault(); }
      else if (e.key === 'ArrowUp') { move(-1); e.preventDefault(); }
      else if (e.key === 'Enter') { choose(here); e.preventDefault(); }
      else if (e.key === 'Escape') { hide(); }
    });
    document.addEventListener('click', function (e) { if (!box.contains(e.target)) hide(); });
    // Tags the server drew get the same cross as the ones added here.
    chosen.addEventListener('click', function (e) {
      var off = e.target.closest ? e.target.closest('[data-multi-off]') : null;
      if (off) off.parentElement.remove();
    });

    function load(q) {
      var req = new XMLHttpRequest();
      req.open('GET', endpoint + '?mode=all&q=' + encodeURIComponent(q));
      req.onload = function () {
        if (req.status !== 200) { hide(); return; }
        draw(JSON.parse(req.responseText));
      };
      req.onerror = hide;
      req.send();
    }

    function taken() {
      var out = {};
      var hidden = chosen.querySelectorAll('input[type="hidden"]');
      for (var i = 0; i < hidden.length; i++) out[hidden[i].value] = true;
      return out;
    }

    function draw(found) {
      var already = taken();
      rows = [];
      for (var i = 0; i < found.length; i++) if (!already[found[i].id]) rows.push(found[i]);
      here = rows.length ? 0 : -1;
      list.innerHTML = '';
      for (var j = 0; j < rows.length; j++) list.appendChild(row(rows[j], j));
      list.hidden = !list.firstChild;
      paint();
    }

    function row(found, i) {
      var d = document.createElement('div');
      d.textContent = found.label;
      d.addEventListener('mousedown', function (e) { e.preventDefault(); choose(i); });
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
      for (var i = 0; i < kids.length; i++) kids[i].className = i === here ? 'hl' : '';
    }

    function choose(i) {
      if (i < 0 || i > rows.length - 1) return;
      add(rows[i].id, rows[i].name);
      text.value = '';
      hide();
      text.focus();
    }

    function add(id, name) {
      var tag = document.createElement('span');
      tag.className = 'chosen-tag';
      var label = document.createElement('span');
      label.textContent = name;
      var hidden = document.createElement('input');
      hidden.type = 'hidden'; hidden.name = field; hidden.value = id;
      var off = document.createElement('button');
      off.type = 'button'; off.className = 'chosen-off';
      off.setAttribute('aria-label', 'Take ' + name + ' off this entry');
      off.textContent = '×';
      off.addEventListener('click', function () { tag.remove(); });
      tag.appendChild(label); tag.appendChild(hidden); tag.appendChild(off);
      chosen.appendChild(tag);
    }

    function hide() { list.hidden = true; rows = []; here = -1; }
  }
})();
