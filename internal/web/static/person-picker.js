// The person picker. It offers matches and never insists: no confirmation step,
// no "did you mean", nothing blocked. Typing a name or a mobile number into the
// one box finds the same person either way.
(function () {
  var boxes = document.querySelectorAll('[data-people]');
  for (var i = 0; i < boxes.length; i++) wire(boxes[i]);

  function wire(box) {
    var text = box.querySelector('[data-people-text]');
    var list = box.querySelector('[data-people-list]');
    var scope = box.getAttribute('data-scope') || '';
    var allowNew = box.getAttribute('data-new') === 'yes';
    var personRow = box.closest('[data-person-row]');
    var fieldScope = personRow || document;
    var department = fieldScope.querySelector('[data-person-department]');
    var mobile = fieldScope.querySelector('[data-person-mobile]');

    text.addEventListener('input', function () { load(text.value); });
    text.addEventListener('focus', function () { load(text.value); });
    document.addEventListener('click', function (e) {
      if (!box.contains(e.target)) list.hidden = true;
    });

    function load(q) {
      var req = new XMLHttpRequest();
      req.open('GET', '/api/people?scope=' + scope + '&q=' + encodeURIComponent(q));
      req.onload = function () {
        if (req.status !== 200) { list.hidden = true; return; }
        draw(JSON.parse(req.responseText));
      };
      req.onerror = function () { list.hidden = true; };
      req.send();
    }

    function draw(found) {
      list.innerHTML = '';
      for (var i = 0; i < found.length; i++) {
        // Somebody who has never taken anything has nothing to bring back, so
        // the returning screen never offers to make a new person.
        if (found[i].new && !allowNew) continue;
        list.appendChild(row(found[i]));
      }
      list.hidden = !list.firstChild;
    }

    function row(person) {
      var d = document.createElement('div');
      d.textContent = person.label;
      if (person.new) d.className = 'new';
      d.addEventListener('mousedown', function (e) {
        e.preventDefault();
        text.value = person.name;
        if (!person.new) {
          if (department) department.value = person.department;
          if (mobile) mobile.value = person.mobile;
        }
        list.hidden = true;
        text.dispatchEvent(new Event('change'));
        // On the returning screen the box is its own little search form: tapping
        // a row should show what that person is holding, not wait for Enter.
        var find = text.form;
        if (find && find.hasAttribute('data-findform')) find.submit();
      });
      return d;
    }
  }
})();
