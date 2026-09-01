// When fewer come back than went out, the screen has to ask what happened to
// the rest - and every one of those sentences carries the number, so they are
// drawn by the server rather than assembled here. This file only asks for the
// page again when the count crosses that line.
(function () {
  var form = document.querySelector('[data-return]');
  if (!form) return;
  var qty = form.querySelector('[data-qty]');
  var total = parseInt(qty.getAttribute('max'), 10);
  var showing = !!form.querySelector('[data-short]');

  qty.addEventListener('change', function () {
    var n = parseInt(qty.value, 10);
    var short = (n >= 1 && n < total);
    if (short === showing) return;
    window.location = '/return/new?q=' + encodeURIComponent(form.q.value) +
      '&holdingIssueId=' + encodeURIComponent(form.holdingIssueId.value) +
      '&productId=' + encodeURIComponent(form.productId.value) +
      '&quantity=' + encodeURIComponent(qty.value);
  });
})();
