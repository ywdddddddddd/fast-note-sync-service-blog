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
  var css = document.createElement('style');
  css.textContent = [
    '.fns-share-menu{position:fixed;z-index:9999;min-width:132px;padding:6px;border-radius:12px;',
    '  background:var(--card,#fff);border:1px solid var(--border,#0000001f);',
    '  box-shadow:0 12px 32px rgba(0,0,0,.16);font-size:.8125rem;display:none}',
    '.fns-share-menu.on{display:block}',
    '.fns-share-menu button{display:block;width:100%;text-align:left;padding:8px 10px;border:0;',
    '  border-radius:8px;background:none;color:var(--foreground,#34322d);font:inherit;cursor:pointer}',
    '.fns-share-menu button:hover{background:var(--muted,#f3f3f3)}',
    '.fns-quote-btn{position:fixed;z-index:9999;padding:6px 12px;border-radius:999px;border:1px solid var(--border,#0000001f);',
    '  background:var(--card,#fff);color:var(--foreground,#34322d);font-size:.75rem;cursor:pointer;',
    '  box-shadow:0 8px 22px rgba(0,0,0,.16);display:none}',
    '.fns-quote-btn.on{display:block}',
    '.fns-quote-btn[disabled]{opacity:.55;cursor:not-allowed}'
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

  // ── 导出 ───────────────────────────────────────────────────────────────
  function save(canvas, name){
    function done(url, revoke){
      var a=document.createElement('a');
      a.href=url; a.download=name;
      document.body.appendChild(a); a.click(); a.remove();
      if(revoke) setTimeout(function(){ URL.revokeObjectURL(url); }, 4000);
    }
    if(canvas.toBlob){
      canvas.toBlob(function(blob){
        if(!blob){ toast('生成失败，请重试'); return; }
        done(URL.createObjectURL(blob), true);
      }, 'image/png');
    } else {
      done(canvas.toDataURL('image/png'), false);
    }
  }

  function toast(msg){
    var d=document.createElement('div');
    d.textContent=msg;
    d.style.cssText='position:fixed;left:50%;bottom:48px;transform:translateX(-50%);z-index:10000;'+
      'padding:9px 16px;border-radius:999px;background:rgba(0,0,0,.82);color:#fff;font-size:.8125rem';
    document.body.appendChild(d);
    setTimeout(function(){ d.remove(); }, 2200);
  }

  function copyLink(){
    var url=location.origin+location.pathname;
    var text=(data.title?data.title+'\n':'')+url;
    if(navigator.clipboard && navigator.clipboard.writeText){
      navigator.clipboard.writeText(text).then(function(){ toast('链接已复制'); }, fallback);
    } else { fallback(); }
    function fallback(){
      var ta=document.createElement('textarea');
      ta.value=text; ta.style.cssText='position:fixed;opacity:0';
      document.body.appendChild(ta); ta.select();
      try{ document.execCommand('copy'); toast('链接已复制'); }catch(e){ toast('复制失败，请手动复制'); }
      ta.remove();
    }
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
        save(canvas, (data.title||'share')+'.png');
      }catch(err){ toast('生成失败：'+err.message); }
    });
  });

  document.addEventListener('click', function(e){
    if(!menu.contains(e.target)) hideMenu();
  });
  window.addEventListener('scroll', hideMenu, {passive:true});

  // ── 划词分享 ───────────────────────────────────────────────────────────
  var qbtn=document.createElement('button');
  qbtn.className='fns-quote-btn';
  qbtn.type='button';
  document.body.appendChild(qbtn);

  var pendingText='';

  function clearQuote(){ qbtn.classList.remove('on'); pendingText=''; }

  document.addEventListener('mouseup', function(e){
    setTimeout(function(){
      var sel=window.getSelection();
      if(!sel || sel.isCollapsed){ clearQuote(); return; }
      var text=(sel.toString()||'').replace(/\s+/g,' ').trim();
      if(!text){ clearQuote(); return; }
      var body=document.querySelector('.post-body');
      if(!body || !body.contains(sel.anchorNode)){ clearQuote(); return; }

      // 超过上限时仍然给出按钮，但写明原因：让读者知道是"选多了"，
      // 而不是以为这个功能坏了。
      var tooLong=text.length>MAX_QUOTE;
      pendingText=tooLong?'':text;
      qbtn.disabled=tooLong;
      qbtn.textContent=tooLong?('最多分享 '+MAX_QUOTE+' 字'):'分享这段';
      qbtn.classList.add('on');
      var rect=sel.getRangeAt(0).getBoundingClientRect();
      var left=Math.min(Math.max(8, rect.left+rect.width/2-qbtn.offsetWidth/2), window.innerWidth-qbtn.offsetWidth-8);
      var top=rect.bottom+10;
      if(top+qbtn.offsetHeight>window.innerHeight-8) top=rect.top-qbtn.offsetHeight-10;
      qbtn.style.left=left+'px'; qbtn.style.top=Math.max(8,top)+'px';
    }, 0);
  });

  qbtn.addEventListener('mousedown', function(e){ e.preventDefault(); });
  qbtn.addEventListener('click', function(){
    if(!pendingText) return;
    var text=pendingText;
    clearQuote();
    toast('正在生成…');
    loadImage(data.avatar).then(function(av){
      try{
        var canvas=drawQuote(text, av, theme());
        save(canvas, '摘录.png');
      }catch(err){ toast('生成失败：'+err.message); }
    });
  });
})();`
}
