/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

(function () {
  document.querySelectorAll('select[data-reveal]').forEach(function (select) {
    var target = document.getElementById(select.getAttribute('data-reveal'));
    if (!target) return;
    var input = target.querySelector('input');
    function sync() {
      var show = select.value === 'new';
      target.hidden = !show;
      if (input) {
        input.required = show;
        if (show) setTimeout(function () { input.focus(); }, 0);
      }
    }
    select.addEventListener('change', sync);
    sync();
  });
  document.querySelectorAll('form').forEach(function (form) {
    form.addEventListener('submit', function (e) {
      var button = e.submitter;
      if (button) button.setAttribute('aria-busy', 'true');
    });
  });
})();
