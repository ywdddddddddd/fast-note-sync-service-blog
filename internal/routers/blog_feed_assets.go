package routers

// Subscribe dialog.
//
// Why a dialog instead of a plain link: /blog/rss.xml is an XML document, and opening it on a
// phone shows the reader source code. Every other outcome is better than that, so the entry point
// explains what to do with the address and copies it in one tap.
//
// The dialog is built in script rather than rendered by the template because it is needed on
// three page types and only ever used once per visit; a hidden block in every template would cost
// markup on every page to serve a control most readers never open.
//
// 订阅弹层。
//
// 为什么是弹层而不是一个普通链接：/blog/rss.xml 是一份 XML 文档，在手机上打开给读者看的是源码。
// 其它任何结果都比这个好，因此入口直接说明地址该怎么用，并一键复制。
//
// 弹层由脚本构建而不是模板渲染，因为它要被三种页面使用，而每次访问至多用到一次；
// 在每个模板里放一段隐藏结构，等于让每一页都为大多数人从不打开的控件付出标记成本。
const blogFeedScript = `
(function () {
  'use strict';

  var wrap = null, card = null, input = null, copyBtn = null, label = null;
  var opener = null;

  function svgClose() {
    var ns = 'http://www.w3.org/2000/svg';
    var s = document.createElementNS(ns, 'svg');
    s.setAttribute('viewBox', '0 0 16 16');
    s.setAttribute('width', '14');
    s.setAttribute('height', '14');
    s.setAttribute('aria-hidden', 'true');
    var p = document.createElementNS(ns, 'path');
    p.setAttribute('d', 'M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.75.75 0 1 1 1.06 1.06L9.06 8l3.22 3.22a.75.75 0 1 1-1.06 1.06L8 9.06l-3.22 3.22a.75.75 0 0 1-1.06-1.06L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06Z');
    s.appendChild(p);
    return s;
  }

  function build(url, title) {
    wrap = document.createElement('div');
    wrap.className = 'feed-pop-wrap';

    card = document.createElement('div');
    card.className = 'feed-pop';
    card.setAttribute('role', 'dialog');
    card.setAttribute('aria-modal', 'true');
    card.setAttribute('aria-label', '订阅地址');

    var x = document.createElement('button');
    x.type = 'button';
    x.className = 'feed-pop-x';
    x.setAttribute('aria-label', '关闭');
    x.appendChild(svgClose());

    var head = document.createElement('p');
    head.className = 'feed-pop-title';
    head.textContent = '订阅《' + title + '》';

    var hint = document.createElement('p');
    hint.className = 'feed-pop-hint';
    hint.textContent = '把这个地址粘进阅读器：Feedly、Inoreader、NetNewsWire、微信读书都可以。新笔记会自动送到你面前。';

    input = document.createElement('input');
    input.className = 'feed-pop-url';
    input.type = 'text';
    input.readOnly = true;
    input.spellcheck = false;
    input.value = url;
    input.setAttribute('aria-label', '订阅地址');
    input.addEventListener('focus', function () { input.select(); });

    copyBtn = document.createElement('button');
    copyBtn.type = 'button';
    copyBtn.className = 'feed-pop-copy';
    copyBtn.textContent = '复制地址';

    label = document.createElement('span');
    label.className = 'feed-pop-done';
    label.textContent = '';

    card.appendChild(x);
    card.appendChild(head);
    card.appendChild(hint);
    card.appendChild(input);
    card.appendChild(copyBtn);

    wrap.appendChild(card);
    document.body.appendChild(wrap);

    x.addEventListener('click', hide);
    wrap.addEventListener('click', function (e) { if (e.target === wrap) hide(); });
    copyBtn.addEventListener('click', copy);
    document.addEventListener('keydown', onKey);
  }

  function onKey(e) {
    if (e.key === 'Escape' || e.key === 'Esc') hide();
  }

  function copy() {
    var v = input.value;
    var ok = function () {
      copyBtn.textContent = '已复制';
      copyBtn.classList.add('is-done');
      window.setTimeout(function () {
        copyBtn.textContent = '复制地址';
        copyBtn.classList.remove('is-done');
      }, 1600);
    };
    var fallback = function () {
      input.focus();
      input.select();
      try {
        if (document.execCommand('copy')) { ok(); }
        else { copyBtn.textContent = '请长按选中'; }
      } catch (err) {
        copyBtn.textContent = '请长按选中';
      }
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(v).then(ok, fallback);
    } else {
      fallback();
    }
  }

  function show(btn) {
    var url = btn.getAttribute('data-feed') || '';
    if (!url) { return; }
    opener = btn;
    if (!wrap) {
      var name = document.title || '';
      var cut = name.indexOf(' · ');
      build(url, cut > 0 ? name.slice(0, cut) : name);
    }
    document.documentElement.classList.add('feed-pop-open');
    wrap.classList.add('is-in');
    window.setTimeout(function () { input.select(); }, 60);
  }

  function hide() {
    document.documentElement.classList.remove('feed-pop-open');
    if (wrap) { wrap.classList.remove('is-in'); }
    if (opener && opener.focus) { opener.focus(); }
    opener = null;
  }

  document.addEventListener('click', function (e) {
    var btn = e.target.closest ? e.target.closest('.topnav-feed') : null;
    if (!btn) { return; }
    e.preventDefault();
    show(btn);
  });
})();
`
