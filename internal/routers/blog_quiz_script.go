package routers

// The chapter-end exercise UI: styles plus one script.
//
// Everything here is written against the same constraints as the rest of the blog:
//   · no build step, no framework, no CDN;
//   · the script is a Go raw string, so it must contain no backticks — a single one would
//     truncate the constant and the compiler would report "unexpected name after top level
//     declaration". This has happened four times in this package.
//   · questions never appear in the HTML. They arrive from POST /blog/quiz/pick.
//   · entrance motion uses translate, never transform: .reveal.is-in already owns transform,
//     and overriding it silently kills the article's own reveal.
//
// 章末练习界面：样式加一段脚本。
//
// 这里的每一行都对着本站既有的约束写：
//   · 无构建步骤、无框架、无 CDN；
//   · 脚本是 Go 原始字符串，因此其中不能出现反引号——一个反引号就会截断常量，
//     编译器报 "unexpected name after top level declaration"。这个包里已经踩过四次。
//   · 题目不出现在 HTML 里，只来自 POST /blog/quiz/pick。
//   · 入场位移用 translate，绝不用 transform：.reveal.is-in 已经占用了 transform，
//     覆盖它会无声地毁掉文章自身的浮现效果。

const blogQuizCSS = `
.quiz{margin:36px 0 8px;padding:22px 0 0;border-top:1px solid var(--line,rgba(120,100,80,.16))}
.quiz-head{display:flex;align-items:baseline;gap:12px;flex-wrap:wrap;margin-bottom:14px}
.quiz-title{font-size:17px;font-weight:600;letter-spacing:.02em}
.quiz-sub{font-size:13px;opacity:.62}
.quiz-actions{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:16px}
.quiz-btn{appearance:none;border:1px solid var(--line,rgba(120,100,80,.28));background:transparent;color:inherit;
  font:inherit;font-size:14px;padding:9px 18px;border-radius:999px;cursor:pointer;transition:background .18s,border-color .18s,translate .18s}
.quiz-btn:hover{background:rgba(120,100,80,.07);translate:0 -1px}
.quiz-btn[disabled]{opacity:.45;cursor:default;translate:none}
.quiz-btn.primary{border-color:rgba(150,110,70,.55);background:rgba(150,110,70,.1)}

.quiz-board{margin:0 0 18px;padding:0;list-style:none;font-size:13px}
.quiz-board li{display:flex;gap:10px;padding:5px 0;border-bottom:1px dashed rgba(120,100,80,.12)}
.quiz-board li:last-child{border-bottom:0}
.quiz-board .r{width:22px;opacity:.5;font-variant-numeric:tabular-nums}
.quiz-board .n{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.quiz-board .s{font-variant-numeric:tabular-nums;opacity:.7}
.quiz-board li.self{color:#a4703c;font-weight:600}

.quiz-stage{margin-top:4px}
.quiz-progress{display:flex;align-items:center;gap:12px;font-size:12px;opacity:.66;margin-bottom:14px}
.quiz-bar{flex:1;height:3px;background:rgba(120,100,80,.14);border-radius:2px;overflow:hidden}
.quiz-bar i{display:block;height:100%;width:0;background:rgba(150,110,70,.75);transition:width .28s ease}
.quiz-q{font-size:16px;line-height:1.7;margin-bottom:16px;white-space:pre-wrap;word-break:break-word}
.quiz-badge{font-size:12px;opacity:.5;margin:-10px 0 16px;letter-spacing:.02em}
.quiz-field{display:flex;gap:10px;margin-top:4px}
.quiz-input.ok{border-color:rgba(90,140,90,.6);background:rgba(90,140,90,.1)}
.quiz-input.bad{border-color:rgba(170,90,80,.6);background:rgba(170,90,80,.09)}
.quiz-opts{display:flex;flex-direction:column;gap:9px}
.quiz-opt{appearance:none;text-align:left;font:inherit;font-size:14.5px;line-height:1.6;color:inherit;
  border:1px solid var(--line,rgba(120,100,80,.22));border-radius:12px;background:transparent;
  padding:11px 15px;cursor:pointer;transition:background .16s,border-color .16s,translate .16s;white-space:pre-wrap}
.quiz-opt:hover{background:rgba(120,100,80,.06);translate:2px 0}
.quiz-opt .k{opacity:.5;margin-right:9px}
.quiz-opt[data-state=sel]{border-color:rgba(150,110,70,.6);background:rgba(150,110,70,.1)}
.quiz-opt[data-state=ok]{border-color:rgba(90,140,90,.6);background:rgba(90,140,90,.11)}
.quiz-opt[data-state=bad]{border-color:rgba(170,90,80,.6);background:rgba(170,90,80,.1)}
.quiz-opt[data-state=miss]{border-color:rgba(90,140,90,.5);border-style:dashed}
.quiz-opt[disabled]{cursor:default;translate:none}

.quiz-note{margin-top:15px;font-size:13.5px;line-height:1.75;padding:13px 15px;border-radius:12px;
  background:rgba(120,100,80,.06);white-space:pre-wrap;word-break:break-word}
.quiz-note.ok{background:rgba(90,140,90,.1)}
.quiz-note.bad{background:rgba(170,90,80,.09)}
.quiz-note .src{display:block;margin-top:8px;font-size:12px;opacity:.6}
.quiz-next{margin-top:16px;display:flex;justify-content:flex-end}

.quiz-recall-q{font-size:15px;line-height:1.7;padding:14px 16px;border-radius:12px;background:rgba(120,100,80,.06);
  margin-bottom:12px;white-space:pre-wrap}
.quiz-reveal{margin-top:10px;font-size:14px;line-height:1.8;padding:14px 16px;border-radius:12px;
  background:rgba(150,110,70,.08);white-space:pre-wrap;word-break:break-word}
.quiz-self{display:flex;gap:10px;margin-top:12px}

.quiz-result{font-size:14px;line-height:1.9}
.quiz-score{font-size:30px;font-weight:600;letter-spacing:.01em;margin-bottom:4px;font-variant-numeric:tabular-nums}
.quiz-score small{font-size:14px;font-weight:400;opacity:.55;margin-left:6px}
.quiz-wrong{margin:12px 0 0;padding-left:18px;font-size:13.5px;line-height:1.8}
.quiz-wrong li{margin-bottom:4px}
.quiz-id{font-size:12px;opacity:.55;margin:-4px 0 14px;display:flex;align-items:center;gap:2px}
.quiz-id-btn{appearance:none;background:none;border:0;padding:0 2px;color:inherit;font:inherit;
  text-decoration:underline;text-underline-offset:2px;cursor:pointer;opacity:.85}
.quiz-id-btn:hover{opacity:1}
.quiz-form{display:flex;gap:10px;margin-top:16px;flex-wrap:wrap}
.quiz-input{flex:1;min-width:200px;font:inherit;font-size:14px;padding:9px 13px;border-radius:10px;
  border:1px solid var(--line,rgba(120,100,80,.3));background:transparent;color:inherit}
.quiz-hint{font-size:12px;opacity:.55;margin-top:8px;line-height:1.6}

@media (max-width:860px){
  .quiz-opt{font-size:14px;padding:10px 13px}
  .quiz-q{font-size:15px}
  .quiz-score{font-size:26px}
}
`

// blogQuizJS returns the exercise script.
//
// Written as a function rather than a package-level constant so the browser gets the exact
// same bytes on every request without a global being mutable, and so it is obvious at the
// call site which template field receives it.
//
// blogQuizJS 返回练习脚本。
//
// 写成函数而不是包级常量：这样每次请求拿到的字节完全一致，又不留一个可被改写的全局，
// 并且在调用处一眼能看出它被塞进哪个模板字段。
func blogQuizJS() string {
	return blogQuizScript
}

const blogQuizScript = `
(function () {
  var root = document.getElementById('fns-quiz');
  if (!root) return;

  var slug = root.getAttribute('data-slug') || '';
  var notePath = root.getAttribute('data-note') || '';
  var stage = document.getElementById('quiz-stage');
  var startBtn = document.getElementById('quiz-start');
  var recallBtn = document.getElementById('quiz-recall');
  if (!stage || !startBtn) return;

  var store = {
    pick: null,
    list: [],
    idx: 0,
    answers: {},
    multi: {},
    started: 0,
    shownAt: 0
  };

  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }

  function post(url, body, cb) {
    var xhr = new XMLHttpRequest();
    xhr.open('POST', url, true);
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function () {
      var data = null;
      try { data = JSON.parse(xhr.responseText); } catch (e) {}
      cb(xhr.status, data);
    };
    xhr.onerror = function () { cb(0, null); };
    xhr.send(JSON.stringify(body));
  }

  function show(node) {
    stage.innerHTML = '';
    stage.appendChild(node);
    stage.hidden = false;
  }

  // ── identity ───────────────────────────────────────────────────────────
  //
  // The email is asked for before the first question, not after the last one. The score is
  // attributed to a person, and asking at the end means a reader who abandons halfway leaves
  // nothing behind while a reader who finishes is asked to type an identity they had no
  // reason to expect. It also makes "who am I on this board" answerable from the first click.
  //
  // 邮箱在开第一题之前问，而不是答完最后一题之后。成绩是要记到某个人名下的：末尾才问，
  // 半途离开的读者什么都没留下，而答完的读者会被要求填一个他此前并不知道要填的身份。
  // 提前问，也让"我在榜上是谁"从第一次点击起就有答案。
  var EMAIL_KEY = 'fns-quiz-email';

  function getEmail() {
    try { return localStorage.getItem(EMAIL_KEY) || ''; } catch (e) { return ''; }
  }

  function setEmail(v) {
    try { localStorage.setItem(EMAIL_KEY, v); } catch (e) {}
    refreshIdentity();
  }

  function maskEmail(e) {
    var at = e.lastIndexOf('@');
    if (at < 1) return '***';
    return e.slice(0, 1) + '***' + e.slice(at);
  }

  function refreshIdentity() {
    var host = startBtn.parentNode;
    var line = document.getElementById('quiz-id');
    var email = getEmail();
    if (!email) {
      if (line) line.remove();
      return;
    }
    if (!line) {
      line = el('div', 'quiz-id');
      line.id = 'quiz-id';
      host.parentNode.insertBefore(line, host.nextSibling);
    }
    line.innerHTML = '';
    line.appendChild(document.createTextNode('身份：' + maskEmail(email) + '　'));
    var swap = el('button', 'quiz-id-btn', '更换');
    swap.type = 'button';
    swap.onclick = function () {
      try { localStorage.removeItem(EMAIL_KEY); } catch (e) {}
      refreshIdentity();
    };
    line.appendChild(swap);
  }

  // ── start ──────────────────────────────────────────────────────────────
  function start() {
    if (!getEmail()) {
      askEmail();
      return;
    }
    beginQuiz();
  }

  function askEmail() {
    var view = el('div');
    view.appendChild(el('div', 'quiz-note',
      '开始前先留一个邮箱，用来把成绩记到你名下。不验证、不外发，榜单上只显示 z***@qq.com 这种打码形式。'));
    var form = el('div', 'quiz-form');
    var input = el('input', 'quiz-input');
    input.type = 'email';
    input.autocomplete = 'email';
    input.placeholder = 'you@example.com';
    var go = el('button', 'quiz-btn primary', '开始测验');
    go.type = 'button';
    go.onclick = function () {
      var v = (input.value || '').trim();
      if (v.indexOf('@') < 1) { input.focus(); return; }
      setEmail(v);
      beginQuiz();
    };
    input.onkeydown = function (e) {
      if (e.key === 'Enter') { e.preventDefault(); go.click(); }
    };
    form.appendChild(input);
    form.appendChild(go);
    view.appendChild(form);
    show(view);
    setTimeout(function () { input.focus(); }, 30);
  }

  function beginQuiz() {
    startBtn.disabled = true;
    startBtn.textContent = '出题中…';
    post('/blog/quiz/pick', { slug: slug }, function (status, data) {
      startBtn.disabled = false;
      startBtn.textContent = '重新测验';
      if (status !== 200 || !data) {
        show(el('div', 'quiz-note bad', '出题失败，请刷新页面重试。'));
        return;
      }
      var pool = (data.objective || []).slice(0, 10);
      if (!pool.length) {
        show(el('div', 'quiz-note', '这篇笔记暂时没有可自动判分的题。'));
        return;
      }
      store.pick = data;
      store.list = pool;
      store.idx = 0;
      store.answers = {};
      store.started = Date.now();
      render();
    });
  }

  function progress() {
    var wrap = el('div', 'quiz-progress');
    var t = el('span', null, (store.idx + 1) + ' / ' + store.list.length);
    var bar = el('div', 'quiz-bar');
    var fill = el('i');
    fill.style.width = ((store.idx) / store.list.length * 100).toFixed(1) + '%';
    bar.appendChild(fill);
    wrap.appendChild(t);
    wrap.appendChild(bar);
    return wrap;
  }

  function letters(s) {
    var out = '';
    s = String(s || '').toUpperCase();
    for (var i = 0; i < s.length; i++) {
      if (s.charAt(i) >= 'A' && s.charAt(i) <= 'H' && out.indexOf(s.charAt(i)) < 0) out += s.charAt(i);
    }
    return out.split('').sort().join('');
  }

  // Mirror of quizNormalizeText in blog_quiz_store.go. Both sides must agree, because the
  // browser judges instantly and the server judges again on submit — a disagreement shows up
  // as a score that changes after the fact, which is worse than either rule alone.
  //
  // 与 blog_quiz_store.go 里的 quizNormalizeText 保持一致。两边必须同规则：浏览器即时判、
  // 服务端提交时再判，规则不一致会表现为"提交后分数变了"，那比只用一边更糟。
  function normText(s) {
    s = String(s || '');
    var out = '';
    for (var i = 0; i < s.length; i++) {
      var ch = s.charAt(i);
      var code = s.charCodeAt(i);
      if (code >= 0xFF01 && code <= 0xFF5E) { ch = String.fromCharCode(code - 0xFF00 + 0x20); }
      if (code === 0x3000) continue;
      if (ch === '％') ch = '%';
      if (/[\s，。、；：（）【】《》“”‘’,.;:()\[\]"'·\-_]/.test(ch)) continue;
      out += ch.toLowerCase();
    }
    return out;
  }

  // Mirror of quizSimilarity in blog_quiz_store.go: longest common subsequence over the
  // longer string. Same 0.75 line as the server.
  //
  // 与 blog_quiz_store.go 的 quizSimilarity 一致：较长串上的最长公共子序列，阈值同为 0.75。
  function simText(a, b) {
    var ra = a.split(''), rb = b.split('');
    if (!ra.length || !rb.length) return 0;
    var prev = new Array(rb.length + 1).fill(0);
    var cur = new Array(rb.length + 1).fill(0);
    for (var i = 1; i <= ra.length; i++) {
      for (var j = 1; j <= rb.length; j++) {
        if (ra[i - 1] === rb[j - 1]) cur[j] = prev[j - 1] + 1;
        else cur[j] = prev[j] >= cur[j - 1] ? prev[j] : cur[j - 1];
      }
      var t = prev; prev = cur; cur = t;
      cur.fill(0);
    }
    var longer = Math.max(ra.length, rb.length);
    return prev[rb.length] / longer;
  }

  function blankOk(mine, answer) {
    var got = normText(mine), want = normText(answer);
    if (!got || !want) return false;
    if (got === want) return true;
    return simText(got, want) >= 0.75;
  }

  function render() {
    var q = store.list[store.idx];
    var view = el('div');
    view.appendChild(progress());
    view.appendChild(el('div', 'quiz-q', q.stem));

    // The exam this question came from, one small line under the stem. Readers deciding
    // whether a question is representative care who set it and when; the answer body does
    // not need that weight.
    //
    // 题目来源考试，题干下方一行小字。读者判断一道题是否有代表性时会在意是谁、哪一年出的，
    // 而正文不需要承担这个重量。
    var prov = [];
    if (q.school) prov.push(q.school);
    if (q.year) prov.push(q.year);
    if (prov.length) view.appendChild(el('div', 'quiz-badge', prov.join(' · ')));

    var isBlank = q.kind === 5;
    var opts = el('div', 'quiz-opts');
    var chosen = [];
    var multi = q.kind === 2;
    var lettersPool = 'ABCDEFGH';

    var note = el('div', 'quiz-note');
    note.hidden = true;
    var nextWrap = el('div', 'quiz-next');
    var confirmBtn = el('button', 'quiz-btn', '确认本题');
    confirmBtn.type = 'button';
    confirmBtn.hidden = !multi;
    confirmBtn.onclick = function () { if (chosen.length) settle(chosen); };
    var nextBtn = el('button', 'quiz-btn primary', store.idx === store.list.length - 1 ? '交卷' : '下一题');
    nextBtn.type = 'button';
    nextBtn.onclick = advance;
    nextBtn.hidden = true;
    nextWrap.appendChild(confirmBtn);
    nextWrap.appendChild(nextBtn);

    function pick(btn, letter) {
      if (!multi) {
        settle([letter]);
        return;
      }
      // Multiple choice needs an explicit confirm: clicking the last option must not
      // auto-submit, because the reader cannot know which click was meant to be the last.
      //
      // 多选题需要显式确认：不能因为点了最后一个选项就自动提交，
      // 因为读者无法预料哪一次点击会被当作最后一次。
      var at = chosen.indexOf(letter);
      if (at >= 0) {
        chosen.splice(at, 1);
        btn.removeAttribute('data-state');
      } else {
        chosen.push(letter);
        btn.setAttribute('data-state', 'sel');
      }
      if (!chosen.length) {
        note.hidden = true;
        return;
      }
      note.hidden = false;
      note.className = 'quiz-note';
      note.textContent = '已选 ' + chosen.slice().sort().join('') + '。多选题请先点“确认本题”。';
    }

    for (var i = 0; i < (q.options || []).length; i++) {
      (function (i, letter) {
        var b = el('button', 'quiz-opt');
        b.type = 'button';
        b.appendChild(el('span', 'k', letter + '.'));
        b.appendChild(document.createTextNode(q.options[i]));
        b.setAttribute('data-letter', letter);
        b.onclick = function () { pick(b, letter); };
        opts.appendChild(b);
      })(i, lettersPool.charAt(i));
    }
    if (!isBlank) view.appendChild(opts);

    if (isBlank) {
      // Hollowed sentence: the reader types the missing span. Enter submits, because this is
      // the one question type where reaching for the mouse breaks the rhythm of recall.
      //
      // 挖空句：读者手打缺的那一段。回车即提交——这是唯一一种用鼠标会打断回忆节奏的题型。
      var field = el('div', 'quiz-field');
      var input = el('input', 'quiz-input quiz-blank');
      input.type = 'text';
      input.autocomplete = 'off';
      input.spellcheck = false;
      input.placeholder = '填出空缺的那一段';
      var send = el('button', 'quiz-btn primary', '提交');
      send.type = 'button';
      send.onclick = function () {
        if (!input.value.trim()) { input.focus(); return; }
        settleBlank(input.value);
      };
      input.onkeydown = function (e) {
        if (e.key === 'Enter') { e.preventDefault(); send.click(); }
      };
      field.appendChild(input);
      field.appendChild(send);
      view.appendChild(field);
      setTimeout(function () { input.focus(); }, 30);
    }

    view.appendChild(note);
    view.appendChild(nextWrap);

    function settle(sel) {
      var got = letters(sel.join(''));
      var want = letters(q.answer);
      var ok = got === want;
      store.answers[q.id] = sel.join('');

      var nodes = opts.querySelectorAll('.quiz-opt');
      for (var j = 0; j < nodes.length; j++) {
        var btn2 = nodes[j];
        btn2.disabled = true;
        var L = btn2.getAttribute('data-letter');
        if (want.indexOf(L) >= 0) btn2.setAttribute('data-state', ok ? 'ok' : 'miss');
        else if (got.indexOf(L) >= 0) btn2.setAttribute('data-state', 'bad');
      }
      note.hidden = false;
      note.className = 'quiz-note ' + (ok ? 'ok' : 'bad');
      note.textContent = (ok ? '对。' : '错，正确答案 ' + want + '。') + (q.explain ? '\n' + q.explain : '');
      if (q.source) {
        var s = el('span', 'src', '出处：' + q.source);
        note.appendChild(s);
      }
      confirmBtn.hidden = true;
      nextBtn.hidden = false;
      nextBtn.focus();
    }

    function settleBlank(raw) {
      var ok = blankOk(raw, q.answer);
      store.answers[q.id] = raw;

      var box = view.querySelector('.quiz-field');
      if (box) {
        var inp = box.querySelector('input');
        var btn = box.querySelector('button');
        if (inp) { inp.disabled = true; inp.className = 'quiz-input quiz-blank ' + (ok ? 'ok' : 'bad'); }
        if (btn) btn.hidden = true;
      }
      note.hidden = false;
      note.className = 'quiz-note ' + (ok ? 'ok' : 'bad');
      note.textContent = (ok ? '对。' : '答案是「' + q.answer + '」。');
      if (q.explain) {
        note.appendChild(document.createElement('br'));
        note.appendChild(document.createTextNode(q.explain));
      }
      if (q.source) {
        note.appendChild(el('span', 'src', '出处：' + q.source));
      }
      nextBtn.hidden = false;
      nextBtn.focus();
    }

    show(view);
  }

  function advance() {
    if (store.idx < store.list.length - 1) {
      store.idx++;
      render();
      return;
    }
    finish();
  }

  // ── finish ─────────────────────────────────────────────────────────────
  function finish() {
    var correct = 0;
    var wrong = [];
    for (var i = 0; i < store.list.length; i++) {
      var q = store.list[i];
      var mine = store.answers[q.id] || '';
      var ok;
      if (q.kind === 5) {
        ok = blankOk(mine, q.answer);
      } else {
        var got = letters(mine);
        ok = got !== '' && got === letters(q.answer);
      }
      if (ok) correct++;
      else wrong.push(q);
    }
    var ms = Date.now() - store.started;
    var rate = store.list.length ? correct / store.list.length : 0;

    var view = el('div', 'quiz-result');
    var score = el('div', 'quiz-score', correct + ' / ' + store.list.length);
    score.appendChild(el('small', null, (rate * 100).toFixed(0) + '%'));
    view.appendChild(score);

    if (wrong.length) {
      var list = el('ol', 'quiz-wrong');
      for (var j = 0; j < wrong.length; j++) {
        list.appendChild(el('li', null, wrong[j].stem));
      }
      view.appendChild(list);
      view.appendChild(el('div', 'quiz-hint', '答错的题已记入错题本，可在这篇笔记重练，或到排行榜页集中回看。'));
    } else {
      view.appendChild(el('div', 'quiz-hint', '全对。'));
    }

    var form = el('div', 'quiz-form');
    var input = el('input', 'quiz-input');
    input.type = 'email';
    input.placeholder = '填邮箱上榜（只用于区分，展示会打码）';
    var saved = '';
    try { saved = localStorage.getItem('fns-quiz-email') || ''; } catch (e) {}
    input.value = saved;
    var submit = el('button', 'quiz-btn primary', '提交成绩');
    submit.type = 'button';
    form.appendChild(input);
    form.appendChild(submit);
    view.appendChild(form);
    view.appendChild(el('div', 'quiz-hint', '成绩由服务端按题库答案重算，改页面改不了名次。邮箱不验证，展示为 z***@qq.com。'));

    var board = el('div');
    view.appendChild(board);

    submit.onclick = function () {
      var email = (input.value || '').trim();
      if (email.indexOf('@') < 1) {
        input.focus();
        return;
      }
      try { localStorage.setItem('fns-quiz-email', email); } catch (e) {}
      submit.disabled = true;
      submit.textContent = '提交中…';
      post('/blog/quiz/submit', {
        email: email,
        notePath: notePath,
        selections: store.answers,
        total: store.list.length,
        ms: ms
      }, function (status, data) {
        submit.disabled = false;
        submit.textContent = '提交成绩';
        if (status !== 200 || !data) {
          board.innerHTML = '';
          board.appendChild(el('div', 'quiz-note bad', '提交失败，请稍后再试。'));
          return;
        }
        board.innerHTML = '';
        var head = el('div', 'quiz-note ok',
          '已记录：' + data.correct + '/' + data.total +
          '（' + (data.rate * 100).toFixed(0) + '%）' +
          (data.totalRank ? '　全站累计 ' + data.totalPoints + ' 题，第 ' + data.totalRank + ' 名' : ''));
        board.appendChild(head);
        renderBoard(board, data.top || []);
      });
    };

    show(view);
  }

  function renderBoard(host, rows) {
    if (!rows.length) return;
    host.appendChild(el('div', 'quiz-hint', '本章榜'));
    var ol = el('ol', 'quiz-board');
    for (var i = 0; i < rows.length; i++) {
      var li = el('li', rows[i].self ? 'self' : '');
      li.appendChild(el('span', 'r', rows[i].rank));
      li.appendChild(el('span', 'n', rows[i].name));
      li.appendChild(el('span', 's', rows[i].correct + '/' + rows[i].total));
      ol.appendChild(li);
    }
    host.appendChild(ol);
  }

  // ── recall ─────────────────────────────────────────────────────────────
  function recall() {
    recallBtn.disabled = true;
    post('/blog/quiz/pick', { slug: slug }, function (status, data) {
      recallBtn.disabled = false;
      if (status !== 200 || !data || !(data.subjective || []).length) {
        show(el('div', 'quiz-note', '这篇笔记暂时没有可对照的背诵条目。'));
        return;
      }
      var pool = data.subjective.slice(0, 12);
      var i = 0;
      function next() {
        if (i >= pool.length) {
          show(el('div', 'quiz-note ok', '这一批背完了。'));
          return;
        }
        var q = pool[i];
        var view = el('div');
        var bar = el('div', 'quiz-progress');
        bar.appendChild(el('span', null, (i + 1) + ' / ' + pool.length));
        view.appendChild(bar);
        view.appendChild(el('div', 'quiz-recall-q', q.stem));
        var acts = el('div', 'quiz-self');
        var flip = el('button', 'quiz-btn primary', '看答案');
        flip.type = 'button';
        acts.appendChild(flip);
        view.appendChild(acts);
        flip.onclick = function () {
          acts.innerHTML = '';
          var rev = el('div', 'quiz-reveal', q.answer);
          if (q.explain) {
            var e2 = el('span', 'src', '　出处：' + q.explain);
            rev.appendChild(e2);
          }
          view.appendChild(rev);
          var yes = el('button', 'quiz-btn', '记住了');
          var no = el('button', 'quiz-btn', '没记住');
          yes.type = 'button';
          no.type = 'button';
          yes.onclick = function () { i++; next(); };
          no.onclick = function () { i++; next(); };
          acts.appendChild(yes);
          acts.appendChild(no);
        };
        show(view);
      }
      next();
    });
  }

  startBtn.onclick = start;
  refreshIdentity();
  if (recallBtn) recallBtn.onclick = recall;
})();
`
