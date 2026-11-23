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
  var chevron = '<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>';
  var check = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>';

  function enhance(select, n) {
    var wrap = document.createElement('div');
    wrap.className = 'dropdown';
    var button = document.createElement('button');
    button.type = 'button';
    button.className = 'dropdown-trigger';
    button.setAttribute('role', 'combobox');
    button.setAttribute('aria-haspopup', 'listbox');
    button.setAttribute('aria-expanded', 'false');
    var label = document.createElement('span');
    label.className = 'dropdown-label';
    button.appendChild(label);
    button.insertAdjacentHTML('beforeend', '<span class="dropdown-chevron">' + chevron + '</span>');
    var list = document.createElement('ul');
    list.className = 'dropdown-menu';
    list.id = 'dropdown-' + n;
    list.setAttribute('role', 'listbox');
    list.hidden = true;
    button.setAttribute('aria-controls', list.id);
    var options = Array.prototype.slice.call(select.options);
    var active = select.selectedIndex;

    options.forEach(function (o, i) {
      var li = document.createElement('li');
      li.id = list.id + '-' + i;
      li.setAttribute('role', 'option');
      li.className = 'dropdown-option';
      li.innerHTML = '<span class="dropdown-text"></span><span class="dropdown-check">' + check + '</span>';
      li.firstChild.textContent = o.textContent;
      li.hidden = o.disabled;
      li.addEventListener('mousedown', function (e) { e.preventDefault(); });
      li.addEventListener('click', function (e) { e.preventDefault(); choose(i); });
      li.addEventListener('mousemove', function () { highlight(i); });
      list.appendChild(li);
    });

    function sync() {
      var current = select.options[select.selectedIndex];
      label.textContent = current.textContent;
      wrap.classList.toggle('is-placeholder', current.disabled);
      Array.prototype.forEach.call(list.children, function (li, i) {
        li.setAttribute('aria-selected', String(i === select.selectedIndex));
      });
    }
    function step(from, dir) {
      var i = from;
      for (var n = 0; n < options.length; n++) {
        i = (i + dir + options.length) % options.length;
        if (!options[i].disabled) return i;
      }
      return from;
    }
    function highlight(i) {
      active = i;
      Array.prototype.forEach.call(list.children, function (li, j) { li.classList.toggle('is-active', j === i); });
      button.setAttribute('aria-activedescendant', list.id + '-' + i);
      list.children[i].scrollIntoView({ block: 'nearest' });
    }
    function open() {
      list.hidden = false;
      wrap.classList.add('is-open');
      button.setAttribute('aria-expanded', 'true');
      var r = button.getBoundingClientRect();
      wrap.classList.toggle('opens-up', window.innerHeight - r.bottom < 220 && r.top > window.innerHeight - r.bottom);
      highlight(options[select.selectedIndex].disabled ? step(select.selectedIndex, 1) : select.selectedIndex);
    }
    function close() {
      list.hidden = true;
      wrap.classList.remove('is-open');
      button.setAttribute('aria-expanded', 'false');
      button.removeAttribute('aria-activedescendant');
    }
    function choose(i) {
      select.selectedIndex = i;
      select.dispatchEvent(new Event('change', { bubbles: true }));
      sync();
      close();
      button.focus();
    }

    button.addEventListener('click', function () { list.hidden ? open() : close(); });
    button.addEventListener('keydown', function (e) {
      var keys = ['ArrowDown', 'ArrowUp', 'Enter', ' '];
      if (list.hidden) {
        if (keys.indexOf(e.key) >= 0) { e.preventDefault(); open(); }
        return;
      }
      if (e.key === 'ArrowDown') { e.preventDefault(); highlight(step(active, 1)); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); highlight(step(active, -1)); }
      else if (e.key === 'Home') { e.preventDefault(); highlight(0); }
      else if (e.key === 'End') { e.preventDefault(); highlight(options.length - 1); }
      else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); choose(active); }
      else if (e.key === 'Escape' || e.key === 'Tab') { close(); }
      else if (e.key.length === 1) {
        for (var k = 0; k < options.length; k++) {
          if (!options[k].disabled && options[k].textContent.toLowerCase().indexOf(e.key.toLowerCase()) === 0) { highlight(k); break; }
        }
      }
    });
    document.addEventListener('mousedown', function (e) { if (!wrap.contains(e.target)) close(); });
    window.addEventListener('resize', close);

    select.classList.add('dropdown-native');
    select.tabIndex = -1;
    select.setAttribute('aria-hidden', 'true');
    select.parentNode.insertBefore(wrap, select);
    wrap.appendChild(button);
    wrap.appendChild(list);
    wrap.appendChild(select);
    var labelEl = select.closest('label');
    if (labelEl) {
      labelEl.addEventListener('click', function (e) { if (e.target === labelEl || e.target.classList.contains('label')) { e.preventDefault(); button.focus(); } });
      var title = labelEl.querySelector('.label');
      if (title) button.setAttribute('aria-label', title.textContent);
    }
    sync();
  }

  document.querySelectorAll('select[data-dropdown]').forEach(enhance);
})();
