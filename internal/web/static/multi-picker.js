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
    var newAt = box.getAttribute('data-new-endpoint') || '';
    var csrf = box.getAttribute('data-csrf') || '';
    var rows = [], here = -1, confirming = '';

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
      // The thing being paid for is not always on the list yet. Adding it is a
      // deliberate press of its own, never something that happens by typing.
      var typed = text.value.trim();
      if (newAt && typed !== '' && !exactly(found, typed)) list.appendChild(newRow(typed));
      list.hidden = !list.firstChild;
      paint();
    }

    function exactly(found, typed) {
      for (var i = 0; i < found.length; i++) {
        if (found[i].name.toLowerCase() === typed.toLowerCase()) return true;
      }
      return false;
    }

    function newRow(typed) {
      var d = document.createElement('div');
      d.className = 'new';
      d.textContent = confirming === typed.toLowerCase()
        ? 'Press again to add "' + typed + '" as a separate product'
        : '+ Add "' + typed + '" as a brand-new product';
      d.addEventListener('mousedown', function (e) { e.preventDefault(); create(typed); });
      return d;
    }

    function create(typed) {
      var body = 'name=' + encodeURIComponent(typed) + '&csrf=' + encodeURIComponent(csrf);
      if (confirming === typed.toLowerCase()) body += '&confirm=yes';
      var req = new XMLHttpRequest();
      req.open('POST', newAt);
      req.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
      req.onload = function () {
        var answer = {};
        try { answer = JSON.parse(req.responseText); } catch (e) { answer = {}; }
        if (answer.id) {
          confirming = '';
          add(answer.id, answer.name);
          text.value = '';
          hide();
          text.focus();
          return;
        }
        if (answer.needsConfirm) confirming = typed.toLowerCase();
        else confirming = '';
        say(answer.error || 'That product could not be added.');
      };
      req.send(body);
    }

    // The refusal goes where the person is looking, in the list itself.
    function say(words) {
      list.innerHTML = '';
      var d = document.createElement('div');
      d.className = 'warn';
      d.textContent = words;
      list.appendChild(d);
      var typed = text.value.trim();
      if (typed !== '') list.appendChild(newRow(typed));
      rows = [];
      here = -1;
      list.hidden = false;
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
