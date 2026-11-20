// Package routers — 分享卡的前端实现。
//
// 为什么整套逻辑都在客户端：卡片要包含封面、头像和站点自有字体，这些浏览器本来就
// 已经加载好了；放到服务端就得再引一套图片合成与中文字体嵌入，而站点连一个中文字体
// 子集都是自己裁的。客户端 canvas 一次绘制就能同时满足"所见即所得"和"零服务端成本"。
//
// 二维码是唯一的例外：它在服务端算（/blog/qr.png）。扫码是一次性、必须正确的动作，
// 而服务端那份编码器可以用参考实现逐位验证，页面里的 JS 做不到。

package routers

// blogShareJS 是分享菜单、划词分享与两张卡片的绘制。
//
// 它不依赖任何框架，也不改模板结构：菜单、划词按钮、样式表全部在运行时创建。
// 这样做的原因是文章页的模板已经很长，而这三样东西都只在用户点击后才存在。
func blogShareJS() string {
	return `(function(){
  var holder = document.getElementById('fns-share-data');
  if (!holder) return;
  var data = {};
  try { data = JSON.parse(holder.textContent || '{}'); } catch (e) { return; }

  var MAX_QUOTE = 120;   // 超过这个字数不出分享按钮：卡片放不下，截断又不诚实

  // ── 主题 ───────────────────────────────────────────────────────────────
  // html.dark 是主题按钮切换的结果；没有显式 class 时跟随系统。
  function isDark(){
    var html = document.documentElement;
    if (html.classList.contains('dark')) return true;
    if (html.classList.contains('light')) return false;
    return !!(window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
  }
  function theme(){
    return isDark()
      ? { bg:'#1c1c1a', fg:'#ededed', muted:'#a1a1a1', line:'#ffffff1f' }
      : { bg:'#ffffff', fg:'#34322d', muted:'#8a8884', line:'#00000014' };
  }

  // ── 样式 ───────────────────────────────────────────────────────────────
  //
  // 颜色全部走 var(--…)：这一套控件在深色与浅色下都要活着，而写死的 rgba 在深色底上
  // 会变成一块看不出边界的灰。--graph-accent 在深色下是 #7aa870，与高亮遮罩同一支颜色，
  // 因此「主按钮」在两种主题里都还是同一个绿。
  var css = document.createElement('style');
  css.textContent = [
    '.fns-share-menu{position:fixed;z-index:9999;min-width:132px;padding:6px;border-radius:12px;',
    '  background:var(--card,#fff);border:1px solid var(--border,#0000001f);',
    '  box-shadow:0 12px 32px rgba(0,0,0,.16);font-size:.8125rem;display:none}',
    '.fns-share-menu.on{display:block}',
    '.fns-share-menu button{display:block;width:100%;text-align:left;padding:8px 10px;border:0;',
    '  border-radius:8px;background:none;color:var(--foreground,#34322d);font:inherit;cursor:pointer}',
    '.fns-share-menu button:hover{background:var(--muted,#f3f3f3)}',

    // 划词工具条：像 iOS/微信那样一条深色胶囊，按钮等宽、图标在上文字在下。
    // 深色底在浅色页面上是「浮起的控件」；在深色页面上它必须再亮一档、并带一圈更明显的边，
    // 否则 rgba(28,28,26) 压在 #1a1a1a 上几乎看不见边界（实测截图就是这样）。
    // 两条规则都写，不靠 color-mix：这条 CSS 是运行时拼出来的，老 Safari 上不可用的语法
    // 会让整条声明失效。
    '.fns-quote-bar{position:fixed;z-index:9999;display:none;align-items:stretch;gap:2px;',
    '  padding:6px;border-radius:12px;background:rgba(28,28,26,.94);',
    '  border:1px solid rgba(255,255,255,.10);box-shadow:0 10px 28px rgba(0,0,0,.34);',
    '  -webkit-backdrop-filter:blur(10px);backdrop-filter:blur(10px);',
    '  -webkit-user-select:none;user-select:none}',
    'html.dark .fns-quote-bar{background:rgba(56,56,52,.97);border-color:rgba(255,255,255,.22);',
    '  box-shadow:0 12px 32px rgba(0,0,0,.62)}',
    // 没有显式 class 时跟随系统：站点其余深色规则都写成这两条并列，工具条也必须如此，
    // 否则「跟随系统 + 深色」的读者拿到的仍是浅色页面那一档的胶囊。
    '@media (prefers-color-scheme:dark){html:not(.light) .fns-quote-bar{background:rgba(56,56,52,.97);',
    '  border-color:rgba(255,255,255,.22);box-shadow:0 12px 32px rgba(0,0,0,.62)}}',
    '.fns-quote-bar.on{display:flex}',
    '.fns-quote-bar button{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:2px;',
    '  min-width:58px;padding:6px 8px;border:0;border-radius:8px;background:none;color:#f2f1ee;',
    '  font:inherit;font-size:.6875rem;line-height:1.15;cursor:pointer;-webkit-tap-highlight-color:transparent}',
    '.fns-quote-bar button svg{width:18px;height:18px}',
    '.fns-quote-bar button:active{background:rgba(255,255,255,.14)}',
    '.fns-quote-bar button[disabled]{opacity:.42;cursor:not-allowed}',

    // 预览层：手机上是「全屏看图」，桌面上是一张居中的浮层。
    '.fns-share-prev{position:fixed;inset:0;z-index:10000;display:flex;flex-direction:column;',
    '  align-items:center;justify-content:center;gap:14px;padding:max(18px,env(safe-area-inset-top,0px)) 16px',
    '  max(18px,env(safe-area-inset-bottom,0px));',
    '  background:rgba(12,12,12,.78);-webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px);',
    '  opacity:0;transition:opacity .22s ease;overflow:auto}',
    '.fns-share-prev.on{opacity:1}',
    '.fns-share-prev-fig{display:flex;align-items:center;justify-content:center;max-width:100%;max-height:72vh}',
    '.fns-share-prev-img{max-width:min(100%,560px);max-height:72vh;width:auto;height:auto;',
    '  border-radius:14px;box-shadow:0 18px 48px rgba(0,0,0,.42);background:#fff;',
    '  -webkit-user-select:auto;user-select:auto;-webkit-touch-callout:default}',
    '.fns-share-prev-bar{display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:center}',
    '.fns-share-prev-btn{display:inline-flex;align-items:center;gap:6px;padding:9px 16px;border-radius:999px;',
    '  border:1px solid rgba(255,255,255,.22);background:rgba(255,255,255,.10);color:#f4f3f0;',
    '  font:inherit;font-size:.8125rem;cursor:pointer;-webkit-tap-highlight-color:transparent}',
    '.fns-share-prev-btn svg{width:16px;height:16px}',
    '.fns-share-prev-btn.primary{background:var(--graph-accent,#4b7a43);border-color:transparent;color:#fff;font-weight:600}',
    '.fns-share-prev-btn:active{transform:translateY(1px)}',
    '.fns-share-prev-hint{margin:0;color:rgba(255,255,255,.62);font-size:.75rem;text-align:center}'
  ].join('\n');
  document.head.appendChild(css);

  // ── 工具：圆角矩形、换行、两端对齐 ──────────────────────────────────────
  function rr(c,x,y,w,h,r){c.beginPath();c.moveTo(x+r,y);c.arcTo(x+w,y,x+w,y+h,r);c.arcTo(x+w,y+h,x,y+h,r);c.arcTo(x,y+h,x,y,r);c.arcTo(x,y,x+w,y,r);c.closePath();}
  function hexA(h,a){h=h.replace('#','');if(h.length===3)h=h.split('').map(function(c){return c+c;}).join('');
    return 'rgba('+parseInt(h.slice(0,2),16)+','+parseInt(h.slice(2,4),16)+','+parseInt(h.slice(4,6),16)+','+a+')';}
  function wrap(ctx,text,widths,maxLines){
    var out=[], line='';
    for(var i=0;i<text.length;i++){
      var ch=text[i], test=line+ch;
      var w=Array.isArray(widths)?widths[Math.min(out.length,widths.length-1)]:widths;
      if(ctx.measureText(test).width>w && line){ out.push(line); line=ch; if(out.length===maxLines) return out; }
      else line=test;
    }
    if(line && out.length<maxLines) out.push(line);
    return out;
  }
  function justify(ctx,ln,x,y,w){
    if(ln.length<2){ ctx.fillText(ln,x,y); return; }
    var nat=ctx.measureText(ln).width, gap=(w-nat)/(ln.length-1), cx=x;
    for(var i=0;i<ln.length;i++){ ctx.fillText(ln[i],cx,y); cx+=ctx.measureText(ln[i]).width+gap; }
  }
  var FONT='"CustomFont","PingFang SC","Microsoft YaHei",sans-serif';

  // ── 二维码 ─────────────────────────────────────────────────────────────
  // 服务端出图；这里只负责等比缩放画进去。加载失败就不画，卡片其余部分照常可用。
  function qrURL(){
    return '/blog/qr.png?t='+encodeURIComponent(location.origin+location.pathname)+'&s=10';
  }
  var qrImg = new Image();
  qrImg.src = qrURL();

  function newCanvas(w,h){
    var c=document.createElement('canvas');
    var dpr=Math.min(window.devicePixelRatio||1,2);
    c.width=w*2; c.height=h*2;       // 固定 2 倍：分享图要经得起微信压缩
    c.style.width=w+'px'; c.style.height=h+'px';
    var ctx=c.getContext('2d'); ctx.setTransform(2,0,0,2,0,0);
    return {c:c,ctx:ctx,w:w,h:h};
  }

  function loadImage(src){
    return new Promise(function(res){
      if(!src) return res(null);
      var img=new Image();
      img.crossOrigin='anonymous';    // 跨源图会让 canvas 污染，导出时直接抛错
      img.onload=function(){res(img);};
      img.onerror=function(){res(null);};
      img.src=src;
    });
  }

  // ── 整篇卡片 1200×750 ──────────────────────────────────────────────────
  function drawArticle(cover, t){
    var o=newCanvas(1200,750), ctx=o.ctx, W=1200, H=750;
    ctx.fillStyle=t.bg; rr(ctx,0,0,W,H,18); ctx.fill();

    ctx.save(); rr(ctx,0,0,W,H,18); ctx.clip();
    if(cover){
      var iw=cover.naturalWidth, ih=cover.naturalHeight;
      var sc=Math.max(W/iw, H*0.95/ih), dw=iw*sc, dh=ih*sc;
      ctx.globalAlpha=isDark()?0.34:0.62;
      ctx.drawImage(cover,(W-dw)/2,-dh*0.06,dw,dh);
      ctx.globalAlpha=1;
    }
    var g=ctx.createLinearGradient(0,0,0,H);
    g.addColorStop(0,hexA(t.bg,0.22)); g.addColorStop(0.38,hexA(t.bg,0.72));
    g.addColorStop(0.56,hexA(t.bg,0.97)); g.addColorStop(1,hexA(t.bg,1));
    ctx.fillStyle=g; ctx.fillRect(0,0,W,H);
    ctx.restore();

    var padX=60, qrS=104, qrX=W-padX-qrS, qrY=H-padX-qrS, textW=qrX-padX-44;
    ctx.textAlign='left'; ctx.textBaseline='alphabetic';

    ctx.fillStyle=t.muted; ctx.font='500 17px '+FONT;
    ctx.fillText(data.date||'', padX, H-268);

    ctx.fillStyle=t.fg; ctx.font='600 48px '+FONT;
    var title=wrap(ctx, data.title||'', textW+90, 1)[0]||'';
    ctx.fillText(title, padX, H-206);

    ctx.fillStyle=t.muted; ctx.font='400 23px '+FONT;
    var lines=wrap(ctx, data.summary||'', textW, 3);
    for(var i=0;i<lines.length;i++){
      var y=H-150+i*40;
      if(i===lines.length-1) ctx.fillText(lines[i],padX,y); else justify(ctx,lines[i],padX,y,textW);
    }

    // 字数与栏目挪到右上角，与标题同一基线、右边缘对齐二维码右边：
    // 卡片右下角留给了二维码，元信息再压在下面会和它抢注意力；右上角是这张卡唯一
    // 还没有被占用的角，而且与左侧的日期形成一条对角基线。
    var meta=[data.words, data.section].filter(Boolean).join(' · ');
    if(meta){
      ctx.textAlign='right';
      ctx.fillStyle=t.fg; ctx.font='600 18px '+FONT;
      ctx.fillText(meta, W-padX, H-206);
      ctx.textAlign='left';
    }

    if(qrImg.complete && qrImg.naturalWidth) ctx.drawImage(qrImg,qrX,qrY,qrS,qrS);
    return o.c;
  }

  // ── 摘录卡片：三档高度 ─────────────────────────────────────────────────
  // 档位按实测行数定：40 字 3 行 / 80 字 6 行 / 120 字 8 行，每档都留了一整行冗余。
  function quoteHeight(n){ return n<=40?980:(n<=80?1120:1280); }

  function drawQuote(text, avatar, t){
    var H=quoteHeight(text.length);
    var o=newCanvas(900,H), ctx=o.ctx, W=900;
    ctx.fillStyle=t.bg; rr(ctx,0,0,W,H,18); ctx.fill();

    var padX=80, textW=W-padX*2, body=46, lineH=76, capSize=104, capGap=14;
    var first=text[0], rest=text.slice(1);

    ctx.font='600 '+capSize+'px '+FONT;
    var capW=ctx.measureText(first).width;
    var indentX=padX+capW+capGap, indentW=textW-capW-capGap;

    ctx.fillStyle=t.fg; ctx.font='400 '+body+'px '+FONT; ctx.textBaseline='alphabetic';
    var lines=wrap(ctx, rest, [indentW,indentW,textW,textW,textW,textW,textW,textW,textW], 9);

    var startY=300;
    for(var i=0;i<lines.length;i++){
      var x=i<2?indentX:padX, w=i<2?indentW:textW, y=startY+i*lineH;
      if(i===lines.length-1) ctx.fillText(lines[i],x,y); else justify(ctx,lines[i],x,y,w);
    }
    ctx.fillStyle=t.fg; ctx.font='600 '+capSize+'px '+FONT;
    ctx.fillText(first, padX, startY+lineH-8);

    var dividerY=H-250, av=56, qrS=112, ay=dividerY+34;
    ctx.strokeStyle=t.line; ctx.lineWidth=1;
    ctx.beginPath(); ctx.moveTo(padX,dividerY); ctx.lineTo(W-padX,dividerY); ctx.stroke();

    if(avatar){
      ctx.save(); ctx.beginPath(); ctx.arc(padX+av/2,ay+av/2,av/2,0,Math.PI*2); ctx.clip();
      ctx.drawImage(avatar,padX,ay,av,av); ctx.restore();
    } else {
      ctx.fillStyle=hexA(t.fg,0.14);
      ctx.beginPath(); ctx.arc(padX+av/2,ay+av/2,av/2,0,Math.PI*2); ctx.fill();
    }

    ctx.textAlign='left';
    ctx.fillStyle=t.fg; ctx.font='600 22px '+FONT;
    ctx.fillText(data.author||'', padX+av+18, ay+26);
    ctx.fillStyle=t.muted; ctx.font='400 17px '+FONT;
    ctx.fillText(wrap(ctx,data.title||'',W-padX*2-av-18-qrS-24,1)[0]||'', padX+av+18, ay+56);

    if(qrImg.complete && qrImg.naturalWidth) ctx.drawImage(qrImg, W-padX-qrS, dividerY+30, qrS, qrS);
    return o.c;
  }

  // ── 导出：JPEG → 预览 → 系统分享 ───────────────────────────────────────
  //
  // iOS 上这张卡原来是一张 2 MB 的「文件卡」：canvas → <a download> 在 iPhone 上只能存进
  // 「文件」，从微信发出去显示的是一个文件气泡而不是图片，系统分享面板里也只剩能吃文件的应用
  //（相册、隔空投送都不出现）。改 JPEG 后体积小一个数量级，再走 navigator.share
  // 直接进系统分享面板，微信、相册、隔空投送都在里面。
  //
  // 为什么先预览而不是直接分享：navigator.share 必须在用户手势里调用，而画卡片要等封面与头像
  // 加载完，那一刻手势早已过期——这正是「点一下就调起分享」在 iOS 上不可靠的原因。
  // 预览层里的「分享」是一次新的点击，手势是新鲜的，分享才真的能弹出来。
  // 预览顺带解决第二个问题：画错了能当场看见，而不是存下来才发现。
  function toJpeg(canvas, cb){
    if(canvas.toBlob){
      canvas.toBlob(function(b){ cb(b); }, 'image/jpeg', 0.92);
      return;
    }
    // 没有 toBlob 的浏览器：dataURL 转二进制。这条路上的体积优势没了，但预览与分享照旧。
    try{
      var d=canvas.toDataURL('image/jpeg', 0.92).split(',')[1];
      var bin=atob(d), arr=new Uint8Array(bin.length);
      for(var i=0;i<bin.length;i++) arr[i]=bin.charCodeAt(i);
      cb(new Blob([arr], {type:'image/jpeg'}));
    }catch(e){ cb(null); }
  }

  // safeName strips what a filesystem refuses. A note titled "第1章：绪论" is legal on iOS but
  // a colon is not legal on Windows, and the reader may well be sharing from a desktop.
  // safeName 去掉文件系统不接受的字符。标题「第1章：绪论」在 iOS 上合法，冒号在 Windows 上不合法，
  // 而读者完全可能从桌面端分享。
  function safeName(s){
    return (s||'分享').replace(/[\\/:*?"<>|]+/g,'-').replace(/\s+/g,' ').trim().slice(0,48)||'分享';
  }

  var prev=null, prevBlob=null, prevName='';

  function closePreview(){
    var node=prev; prev=null;
    if(!node) return;
    node.classList.remove('on');
    setTimeout(function(){ if(node.parentNode) node.parentNode.removeChild(node); }, 220);
  }

  function downloadBlob(blob, name){
    var url=URL.createObjectURL(blob);
    var a=document.createElement('a');
    a.href=url; a.download=name+'.jpg';
    document.body.appendChild(a); a.click(); a.remove();
    setTimeout(function(){ URL.revokeObjectURL(url); }, 8000);
  }

  function showPreview(blob, name){
    closePreview();
    prevBlob=blob; prevName=name;

    var url=URL.createObjectURL(blob);
    var box=document.createElement('div');
    box.className='fns-share-prev';

    var fig=document.createElement('div');
    fig.className='fns-share-prev-fig';
    var img=document.createElement('img');
    img.className='fns-share-prev-img';
    img.alt='分享卡片预览';
    img.src=url;
    fig.appendChild(img);

    var bar=document.createElement('div');
    bar.className='fns-share-prev-bar';

    function mk(cls, label, icon){
      var b=document.createElement('button');
      b.type='button'; b.className='fns-share-prev-btn'+(cls?' '+cls:'');
      b.innerHTML=icon+'<span>'+label+'</span>';
      bar.appendChild(b);
      return b;
    }

    var ICON_COPY='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M15 5H6a2 2 0 0 0-2 2v9"/></svg>';
    var ICON_SAVE='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3v12"/><path d="m7 11 5 5 5-5"/><path d="M4 20h16"/></svg>';

    var shareBtn=mk('primary','分享',
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3v13"/><path d="m8 7 4-4 4 4"/><path d="M5 14v5a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-5"/></svg>');
    var saveBtn=mk('','保存',ICON_SAVE);
    var closeBtn=mk('','关闭',
      '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 6l12 12M18 6 6 18"/></svg>');

    var hint=document.createElement('p');
    hint.className='fns-share-prev-hint';
    hint.textContent='长按图片也可以直接保存或转发';

    box.appendChild(fig);
    box.appendChild(bar);
    box.appendChild(hint);
    document.body.appendChild(box);
    prev=box;
    // 下一帧再加 on：过渡要有起始态，否则预览层是「跳」出来的。
    requestAnimationFrame(function(){ box.classList.add('on'); });

    function doSave(){
      downloadBlob(blob, name);
      toast(navigator.share ? '已保存为图片' : '已下载图片');
    }

    shareBtn.addEventListener('click', function(){
      var file=null;
      try{ file=new File([blob], name+'.jpg', {type:'image/jpeg'}); }catch(e){ file=null; }
      var canFiles=!!(file && navigator.canShare && navigator.share && navigator.canShare({files:[file]}));
      if(!canFiles){
        // 桌面浏览器大多不支持文件分享。退回保存，而不是让按钮看起来没反应。
        doSave();
        return;
      }
      navigator.share({
        files:[file],
        title:data.title||'',
        text:data.title||''
      }).catch(function(err){
        if(err && err.name==='AbortError') return;   // 用户自己取消，不是失败
        doSave();
      });
    });

    saveBtn.addEventListener('click', doSave);
    closeBtn.addEventListener('click', closePreview);
    box.addEventListener('click', function(e){ if(e.target===box) closePreview(); });
    img.addEventListener('load', function(){ setTimeout(function(){ URL.revokeObjectURL(url); }, 0); });
  }

  // present draws nothing itself: it turns the canvas into a JPEG and puts the preview up.
  // present 不画任何东西：把 canvas 变成 JPEG 并弹出预览。
  function present(canvas, name){
    toJpeg(canvas, function(blob){
      if(!blob){ toast('生成失败，请重试'); return; }
      showPreview(blob, safeName(name));
    });
  }

  function toast(msg){
    var d=document.createElement('div');
    d.textContent=msg;
    d.setAttribute('role','status');
    d.style.cssText='position:fixed;left:50%;bottom:calc(5.4rem + env(safe-area-inset-bottom,0px));'+
      'transform:translateX(-50%);z-index:10001;max-width:min(90vw,26rem);text-align:center;'+
      'padding:9px 16px;border-radius:999px;background:rgba(0,0,0,.82);color:#fff;font-size:.8125rem';
    document.body.appendChild(d);
    setTimeout(function(){ d.remove(); }, 2200);
  }

  function copyText(text, ok, fail){
    function fallback(){
      var ta=document.createElement('textarea');
      ta.value=text; ta.setAttribute('readonly','');
      ta.style.cssText='position:fixed;top:0;left:-9999px;opacity:0';
      document.body.appendChild(ta); ta.select();
      ta.setSelectionRange(0, text.length);
      var done=false;
      try{ done=document.execCommand('copy'); }catch(e){ done=false; }
      ta.remove();
      if(done) ok(); else fail();
    }
    if(navigator.clipboard && navigator.clipboard.writeText){
      navigator.clipboard.writeText(text).then(ok, fallback);
    } else { fallback(); }
  }

  function copyLink(){
    var url=location.origin+location.pathname;
    var text=(data.title?data.title+'\n':'')+url;
    copyText(text, function(){ toast('链接已复制'); }, function(){ toast('复制失败，请手动复制'); });
  }

  // ── 分享菜单 ───────────────────────────────────────────────────────────
  var menu=document.createElement('div');
  menu.className='fns-share-menu';
  menu.innerHTML='<button data-k="link">复制链接</button><button data-k="card">生成卡片</button>';
  document.body.appendChild(menu);

  var shareBtn=document.querySelector('[data-act="share"]');
  function hideMenu(){ menu.classList.remove('on'); }

  if(shareBtn){
    shareBtn.addEventListener('click', function(e){
      e.preventDefault(); e.stopImmediatePropagation();
      if(menu.classList.contains('on')){ hideMenu(); return; }
      var r=shareBtn.getBoundingClientRect();
      menu.classList.add('on');
      var mw=menu.offsetWidth, mh=menu.offsetHeight;
      var left=Math.min(Math.max(8, r.left), window.innerWidth-mw-8);
      var top=r.top-mh-8;
      if(top<8) top=r.bottom+8;
      menu.style.left=left+'px'; menu.style.top=top+'px';
    }, true);
  }

  menu.addEventListener('click', function(e){
    var k=e.target && e.target.getAttribute('data-k');
    if(!k) return;
    hideMenu();
    if(k==='link'){ copyLink(); return; }
    toast('正在生成…');
    Promise.all([loadImage(data.cover), loadImage(data.avatar)]).then(function(res){
      try{
        var canvas=drawArticle(res[0], theme());
        present(canvas, data.title||'分享');
      }catch(err){ toast('生成失败：'+err.message); }
    });
  });

  document.addEventListener('click', function(e){
    if(!menu.contains(e.target)) hideMenu();
  });
  window.addEventListener('scroll', hideMenu, {passive:true});

  // ── 划词工具条 ─────────────────────────────────────────────────────────
  //
  // 原来只监听 mouseup，而手机上的选词根本不产生 mouseup（长按选词走的是 touch 与
  // selectionchange），所以这个功能在手机上「完全不可用」而不是偶发失灵。
  // 现在两个入口都在：selectionchange 覆盖长按与拖选（两端都触发），mouseup 覆盖桌面鼠标，
  // 并且用一次 250ms 的合流把两个入口的重复触发压成一次。
  //
  // 松手之后手机上通常紧跟着一次「点击」（取消选区）。工具条在 pointerdown 时自保：
  // 它在选区之外，浏览器的默认行为会清掉选区、连带把工具条藏起来，于是按钮还没被点到就消失了。
  // 在 pointerdown 上 preventDefault 才让那一次点击落在按钮上。
  var qbar=document.createElement('div');
  qbar.className='fns-quote-bar';
  qbar.setAttribute('role','toolbar');
  qbar.setAttribute('aria-label','划词操作');
  qbar.innerHTML=
    '<button type="button" data-q="copy"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M15 5H6a2 2 0 0 0-2 2v9"/></svg><span>复制</span></button>'+
    '<button type="button" data-q="img"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="m4 17 5-5 4 4 3-2 4 4"/></svg><span>分享图片</span></button>';
  document.body.appendChild(qbar);

  var pendingText='';
  var lastSelRect=null;
  var rafPending=0;

  function clearQuote(){ qbar.classList.remove('on'); pendingText=''; }

  function placeBar(rect){
    lastSelRect=rect;
    var bw=qbar.offsetWidth||160, bh=qbar.offsetHeight||44;
    var left=Math.min(Math.max(8, rect.left+rect.width/2-bw/2), window.innerWidth-bw-8);
    // Above the selection is the phone convention (iOS 的编辑菜单在选区上方)，space allowing.
    // 选区上方是手机的惯例（iOS 的编辑菜单就在上方），空间够时优先。
    var top=rect.top-bh-10;
    if(top<8) top=rect.bottom+10;
    // Both candidates can be off-screen when the selection itself is outside the viewport — a
    // selection made before a viewport change, or one kept alive while the page scrolled. Without
    // this clamp the bar lands at a negative top (measured: -1282px on a phone-sized viewport) and
    // is simply invisible, which looks exactly like the feature being broken again.
    //
    // 当选区本身在视口外时，两个候选位置都会落到屏幕外——例如视口改变之前做的选区，
    // 或者页面滚动后仍然保留的选区。没有这个钳制，工具条会落在负的 top（实测手机尺寸下
    // −1282px），完全看不见，看起来就是「功能又坏了」。
    top=Math.min(Math.max(8, top), Math.max(8, window.innerHeight-bh-8));
    qbar.style.left=left+'px';
    qbar.style.top=top+'px';
  }

  function evaluateSelection(){
    var sel=window.getSelection();
    if(!sel || sel.rangeCount===0 || sel.isCollapsed){ clearQuote(); return; }
    var text=(sel.toString()||'').replace(/\s+/g,' ').trim();
    if(!text){ clearQuote(); return; }
    var body=document.querySelector('.post-body');
    if(!body || !body.contains(sel.anchorNode)){ clearQuote(); return; }

    // 工具条自己的按钮也在 DOM 里；点击它们会改选区，不该被当成一次新的选词。
    // 工具条自己的按钮也在 DOM 里；点击它们会改选区，不该被当成一次新的选词。
    if(qbar.contains(sel.anchorNode)){ return; }

    // 超过上限时仍然显示工具条，但把「分享图片」标成不可用并写明原因：
    // 让读者知道是「选多了」，而不是以为这个功能坏了。
    var tooLong=text.length>MAX_QUOTE;
    pendingText=text;
    var imgBtn=qbar.querySelector('[data-q="img"]');
    if(imgBtn){
      imgBtn.disabled=tooLong;
      imgBtn.title=tooLong?('摘录卡最多 '+MAX_QUOTE+' 字，当前 '+text.length+' 字'):'';
      imgBtn.querySelector('span').textContent=tooLong?'太长':'分享图片';
    }
    var rect=sel.getRangeAt(0).getBoundingClientRect();
    if(!rect || (!rect.width && !rect.height)){ clearQuote(); return; }
    qbar.classList.add('on');
    placeBar(rect);
  }

  function scheduleEvaluate(){
    if(rafPending) return;
    rafPending=setTimeout(function(){ rafPending=0; evaluateSelection(); }, 60);
  }

  // selectionchange 是主入口：拖选、双击、长按选词在两端都会触发它。
  // selectionchange 是主入口：拖选、双击、长按选词在两端都会触发它。
  document.addEventListener('selectionchange', scheduleEvaluate);

  // mouseup 与 touchend 补一次：某些浏览器在一次手势结束时不再发 selectionchange，
  // 选区却已经变了。两者都走同一段延迟合流，不会让工具条闪两次。
  document.addEventListener('mouseup', scheduleEvaluate);
  document.addEventListener('touchend', function(){
    // 松手后浏览器还要几百毫秒才把选区定下来，尤其是长按选词。
    // 松手后浏览器还要几百毫秒才把选区定下来，尤其是长按选词。
    setTimeout(evaluateSelection, 120);
  }, {passive:true});

  // 点工具条上的按钮不能让选区先被清掉。
  qbar.addEventListener('pointerdown', function(e){ e.preventDefault(); });
  qbar.addEventListener('mousedown', function(e){ e.preventDefault(); });
  qbar.addEventListener('touchstart', function(e){
    // 只拦工具条自身，不拦里面的按钮——否则按钮收不到 click。
    // 只拦工具条自身，不拦里面的按钮——否则按钮收不到 click。
    if(e.target===qbar) e.preventDefault();
  }, {passive:false});

  // 点别处就收起。用 pointerdown 而不是 click：手机上「点一下取消选区」的第一次触摸
  // 就应该收掉工具条，等 click 已经太晚。
  document.addEventListener('pointerdown', function(e){
    if(qbar.classList.contains('on') && !qbar.contains(e.target)) clearQuote();
  });
  window.addEventListener('scroll', function(){ if(qbar.classList.contains('on')) clearQuote(); }, {passive:true});

  qbar.addEventListener('click', function(e){
    var btn=e.target && e.target.closest ? e.target.closest('[data-q]') : null;
    if(!btn || btn.disabled) return;
    var kind=btn.getAttribute('data-q');
    var text=pendingText;
    if(!text) return;

    if(kind==='copy'){
      copyText(text, function(){ toast('已复制所选内容'); }, function(){ toast('复制失败，请手动复制'); });
      clearQuote();
      return;
    }

    clearQuote();
    toast('正在生成…');
    loadImage(data.avatar).then(function(av){
      try{
        var canvas=drawQuote(text, av, theme());
        present(canvas, '摘录');
      }catch(err){ toast('生成失败：'+err.message); }
    });
  });
})();`
}
