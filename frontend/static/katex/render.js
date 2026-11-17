/*
 * Typeset the math placeholders produced by the server.
 *
 * The server marks formulas as .math-inline / .math-block and leaves the LaTeX source inside.
 * KaTeX is the only thing this file needs; without it (or if a formula is malformed) the source
 * remains readable, which is the same graceful degradation the stylesheet already provides.
 */
(function () {
  'use strict';

  function renderMath() {
    if (!window.katex) return;
    var nodes = document.querySelectorAll('.math-inline,.math-block');
    for (var i = 0; i < nodes.length; i++) {
      var el = nodes[i];
      if (el.getAttribute('data-katex') === '1') continue;
      var tex = el.textContent || '';
      if (!tex) continue;
      try {
        window.katex.render(tex, el, {
          displayMode: el.classList.contains('math-block'),
          throwOnError: false,
          strict: false,
          trust: false
        });
        el.setAttribute('data-katex', '1');
      } catch (e) {
        /* Leave the source in place. */
      }
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', renderMath);
  } else {
    renderMath();
  }
})();
