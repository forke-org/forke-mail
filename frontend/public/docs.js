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
  var copyIcon = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
  var doneIcon = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2.5" aria-hidden="true"><path d="M20 6 9 17l-5-5"/></svg>';

  function copy(text, button) {
    var done = function () {
      button.classList.add('is-copied');
      setTimeout(function () { button.classList.remove('is-copied'); }, 1400);
    };
    if (navigator.clipboard) navigator.clipboard.writeText(text).then(done, function () {});
  }

  document.querySelectorAll('.docs pre').forEach(function (pre) {
    var button = document.createElement('button');
    button.type = 'button';
    button.className = 'code-copy';
    button.setAttribute('aria-label', 'Copy code');
    button.innerHTML = '<span class="i-copy">' + copyIcon + '</span><span class="i-done">' + doneIcon + '</span>';
    button.addEventListener('click', function () { copy(pre.querySelector('code').innerText.replace(/\n$/, ''), button); });
    pre.appendChild(button);
  });

  document.querySelectorAll('[data-copy]').forEach(function (button) {
    button.addEventListener('click', function () { copy(button.getAttribute('data-copy'), button); });
  });

  var links = {};
  document.querySelectorAll('.docs-toc a').forEach(function (a) { links[a.getAttribute('href').slice(1)] = a; });
  var headings = Array.prototype.slice.call(document.querySelectorAll('.docs h2[id], .docs h3[id]'));
  var current = null;
  function spy() {
    var line = window.innerHeight * 0.25;
    var active = headings[0];
    for (var i = 0; i < headings.length; i++) {
      if (headings[i].getBoundingClientRect().top - line <= 0) active = headings[i];
    }
    if (active && active !== current) {
      if (current && links[current.id]) links[current.id].classList.remove('is-active');
      current = active;
      var link = links[active.id];
      if (link) {
        link.classList.add('is-active');
        var toc = document.querySelector('.docs-toc');
        var r = link.getBoundingClientRect(), t = toc.getBoundingClientRect();
        if (r.top < t.top || r.bottom > t.bottom) link.scrollIntoView({ block: 'nearest' });
      }
    }
  }
  window.addEventListener('scroll', spy, { passive: true });
  spy();

  var reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (!reduce && 'IntersectionObserver' in window) {
    var blocks = document.querySelectorAll('.docs > *');
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) { e.target.classList.add('is-in'); io.unobserve(e.target); }
      });
    }, { rootMargin: '0px 0px -8% 0px' });
    blocks.forEach(function (el) {
      if (el.getBoundingClientRect().top > window.innerHeight) { el.classList.add('reveal'); io.observe(el); }
    });
  }

  document.querySelectorAll('.docs h2 .anchor, .docs h3 .anchor').forEach(function (a) {
    a.addEventListener('click', function () {
      var url = location.origin + location.pathname + a.getAttribute('href');
      if (navigator.clipboard) navigator.clipboard.writeText(url).catch(function () {});
    });
  });
})();
