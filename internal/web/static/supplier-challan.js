(function () {
  var boxes = document.querySelectorAll('[data-supplier-challan]');
  for (var i = 0; i < boxes.length; i++) wire(boxes[i]);

  function wire(box) {
    var row = box.closest('fieldset') || box.closest('form');
    var party = row && row.querySelector('[data-values][data-kind="party"]');
    var partyID = party && party.querySelector('[data-values-id]');
    var text = box.querySelector('[data-challan-text]');
    var id = box.querySelector('[data-challan-id]');
    var list = box.querySelector('[data-challan-results]');
    if (!partyID || !text || !id || !list) return;
    text.addEventListener('input', function () { id.value = ''; load(); });
    text.addEventListener('focus', load);
    if (party) party.addEventListener('valuepicked', function () { id.value = ''; text.value = ''; hide(); });
    function load() {
      if (!partyID.value) { hide(); return; }
      var req = new XMLHttpRequest();
      req.open('GET', '/api/supplier-challans?partyId=' + encodeURIComponent(partyID.value) + '&q=' + encodeURIComponent(text.value));
      req.onload = function () {
        if (req.status !== 200) { hide(); return; }
        var rows = JSON.parse(req.responseText);
        list.innerHTML = '';
        for (var n = 0; n < rows.length; n++) list.appendChild(choice(rows[n]));
        list.hidden = !list.firstChild;
      };
      req.onerror = hide;
      req.send();
    }
    function choice(found) {
      var d = document.createElement('div');
      d.textContent = found.label;
      d.addEventListener('mousedown', function (e) {
        e.preventDefault(); id.value = found.id; text.value = found.value; hide();
      });
      return d;
    }
    function hide() { list.hidden = true; list.innerHTML = ''; }
  }
})();
