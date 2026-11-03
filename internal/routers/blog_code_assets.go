package routers

// Code blocks are the one place where the rendered body is genuinely bare: ByTemd emits
// `<pre><code class="language-x">` and nothing else. The reference site gets a frame, a language
// label, a line-number gutter and a copy button for free from Shiki; this site has no build step,
// so the same four things are added in the browser, on top of markup that already works without
// any script.
//
// 代码块是渲染结果里唯一真正裸着的地方：ByTemd 只输出 `<pre><code class="language-x">`。
// 参照站从 Shiki 免费得到外框、语言标签、行号槽与复制按钮；本站没有构建步骤，
// 所以这四样东西在浏览器里补上，而它们叠加的标记本身在没有任何脚本时依然可用。

// blogCodeJS returns the enhancer.
//
// The line numbers are a separate column rather than injected spans: the code element's text is
// left exactly as rendered, so a future syntax highlighter that emits nested spans keeps working.
// Rewriting the code element would make this script and any highlighter fight over the same nodes.
//
// blogCodeJS 返回增强脚本。
//
// 行号是一列独立的节点，而不是塞进代码里的 span：code 元素的文本保持渲染时的原样，
// 于是将来接入语法高亮（它会输出嵌套 span）也不会被这段脚本破坏。
// 改写 code 元素会让这个脚本和任何高亮器争夺同一批节点。
func blogCodeJS() string {
	return `(function () {
  var blocks = document.querySelectorAll('.post-body pre');
  if (!blocks.length) { return; }

  function langOf(pre) {
    var code = pre.querySelector('code');
    if (!code) { return ''; }
    var m = /(?:^|\s)language-([\w+#-]+)/.exec(code.className || '');
    return m ? m[1] : '';
  }

  function label(lang) {
    if (!lang) { return 'CODE'; }
    var map = { bash: 'BASH', sh: 'SHELL', shell: 'SHELL', js: 'JS', javascript: 'JS',
      ts: 'TS', typescript: 'TS', py: 'PY', python: 'PY', go: 'GO', golang: 'GO',
      json: 'JSON', yaml: 'YAML', yml: 'YAML', sql: 'SQL', md: 'MD', markdown: 'MD',
      html: 'HTML', css: 'CSS', xml: 'XML', java: 'JAVA', kotlin: 'KOTLIN', swift: 'SWIFT' };
    return map[lang.toLowerCase()] || lang.toUpperCase();
  }

  function enhance(pre) {
    if (pre.parentElement && pre.parentElement.classList.contains('code-body')) { return; }
    var code = pre.querySelector('code');
    if (!code) { return; }
    var text = code.textContent || '';

    var wrap = document.createElement('div');
    wrap.className = 'code-block';

    var head = document.createElement('div');
    head.className = 'code-head';
    var tag = document.createElement('span');
    tag.className = 'code-lang';
    tag.textContent = label(langOf(pre));
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'code-copy';
    btn.textContent = '复制';
    btn.setAttribute('aria-label', '复制这段代码');
    head.appendChild(tag);
    head.appendChild(btn);

    var body = document.createElement('div');
    body.className = 'code-body';

    // One line of text per entry; a trailing newline is not a line.
    // 每条对应一行；结尾换行不算一行。
    var lines = text.split('\n');
    if (lines.length && lines[lines.length - 1] === '') { lines.pop(); }
    var gutter = document.createElement('div');
    gutter.className = 'code-ln';
    gutter.setAttribute('aria-hidden', 'true');
    var nums = [];
    for (var i = 1; i <= lines.length; i++) { nums.push(i); }
    gutter.textContent = nums.join('\n');

    pre.parentNode.insertBefore(wrap, pre);
    body.appendChild(gutter);
    body.appendChild(pre);
    wrap.appendChild(head);
    wrap.appendChild(body);

    btn.addEventListener('click', function () {
      var done = function (ok) {
        btn.textContent = ok ? '已复制' : '请长按选中';
        btn.classList.toggle('is-done', ok);
        setTimeout(function () {
          btn.textContent = '复制';
          btn.classList.remove('is-done');
        }, 1600);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(function () { done(true); }, function () { done(false); });
        return;
      }
      // executeCommand fallback for browsers and webviews without the async clipboard.
      // 没有异步剪贴板时的兜底。
      try {
        var ta = document.createElement('textarea');
        ta.value = text;
        ta.setAttribute('readonly', '');
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        var ok = document.execCommand('copy');
        document.body.removeChild(ta);
        done(ok);
      } catch (e) { done(false); }
    });
  }

  Array.prototype.forEach.call(blocks, enhance);
})();`
}
