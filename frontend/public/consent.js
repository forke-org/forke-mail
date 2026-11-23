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
    var select = form.querySelector('select[name="workspace_id"]');
    var error = document.getElementById('project-error');
    if (select) select.addEventListener('change', function () {
      if (error) error.hidden = true;
      var wrap = select.closest('.dropdown');
      if (wrap) wrap.classList.remove('is-invalid');
    });
    form.addEventListener('submit', function (e) {
      var button = e.submitter;
      var allowing = !button || button.value === 'allow';
      if (allowing && select && !select.value) {
        e.preventDefault();
        if (error) error.hidden = false;
        var wrap = select.closest('.dropdown');
        if (wrap) {
          wrap.classList.add('is-invalid');
          var trigger = wrap.querySelector('.dropdown-trigger');
          if (trigger) trigger.focus();
        }
        return;
      }
      if (!allowing) {
        form.querySelectorAll('[required]').forEach(function (el) { el.required = false; });
      }
      if (button) button.setAttribute('aria-busy', 'true');
    });
  });
})();
