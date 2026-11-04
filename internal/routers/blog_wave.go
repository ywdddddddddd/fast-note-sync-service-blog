package routers

// The dot grid becomes a surface that keeps moving.
//
// The static grid is a CSS radial-gradient repeating every 20px, and it stays exactly where it is
// for narrow screens — a WebGL canvas that re-renders the whole viewport every frame is a bad trade
// on a phone, and the grid reads fine at that size anyway. Above 861px it is replaced by a shader
// that draws the same 20px pitch and lights the dots where a wave passes.
//
// The waves are self-touching: nothing emits them but the page itself. A backdrop that only reacts
// to the pointer is motionless whenever the reader is reading, which is most of the time — so the
// surface is given its own slow random heartbeat instead, at random positions, forever.
//
// 点阵变成一片永不静止的表面。
//
// 静态网格是一条每 20px 重复的 CSS 径向渐变，它在窄屏上原样保留——每帧重绘整块视口的 WebGL
// 画布在手机上是笔亏本买卖，而网格在那个尺寸下本来就够看。861px 以上才换成着色器：
// 同样的 20px 间距，只在波经过的地方点亮那些点。
//
// 波是「自触」的：除了页面自己，没有别的东西发出它。一片只在指针动时才反应的背景，
// 在读者真正阅读的时候就是静止的——而那占据了绝大部分时间。所以这片表面有自己的、
// 缓慢随机的脉搏：随机位置、永不停歇。
const blogWaveJS = `
(function () {
  'use strict';

  // Only the two pages that are read rather than scanned. A list page's background competing with
  // fifteen cover images is noise; a card page has room for it.
  //
  // 只给两个「用来读」而不是「用来扫」的页面。列表页的背景与十五张封面抢注意力是噪音，
  // 名片页有容得下它的空白。
  var body = document.body;
  if (!body) { return; }
  if (!body.classList.contains('is-home') && !body.classList.contains('is-page')) { return; }

  var host = document.querySelector('.backdrop');
  if (!host) { return; }

  var desk = window.matchMedia('(min-width: 861px)');
  var calm = window.matchMedia('(prefers-reduced-motion: reduce)');
  if (!desk.matches || calm.matches) { return; }

  var VERT = [
    'attribute vec2 aPos;',
    'void main(){ gl_Position = vec4(aPos, 0.0, 1.0); }'
  ].join('\n');

  // The dot is drawn analytically rather than sampled from a texture: the grid is regular, so the
  // distance to the nearest cell centre is all a fragment needs, and that stays crisp at any DPR
  // without a single texel of storage.
  //
  // 点是解析式画出来的，不是从纹理采样：网格是规则的，因此一个片元只需要「到最近格心的距离」，
  // 而这样在任意 DPR 下都锐利，且不占一个纹素。
  var FRAG = [
    'precision mediump float;',
    'uniform vec2 uRes;',
    'uniform float uDpr;',
    'uniform vec3 uDot;',
    'uniform float uPitch;',
    'uniform float uTime;',
    'uniform vec3 uWave[6];',
    'uniform int uCount;',
    'void main() {',
    '  vec2 px = gl_FragCoord.xy / uDpr;',
    '  px.y = uRes.y - px.y;',                 // WebGL 的原点在左下，翻成与 CSS 一致的从上往下
    '  vec2 cell = mod(px, uPitch) - uPitch * 0.5;',
    '  float d = length(cell);',
    '  float dotMask = 1.0 - smoothstep(0.62, 1.30, d);',
    '  float amp = 0.0;',
    '  for (int i = 0; i < 6; i++) {',
    '    if (i < uCount) {',
    '      float age = uTime - uWave[i].z;',
    '      if (age >= 0.0 && age <= 2.4) {',
    '        float dist = length(px - uWave[i].xy);',
    '        float band = (dist - age * 300.0) / 110.0;',
    '        amp += exp(-band * band) * exp(-age * 1.15);',
    '      }',
    '    }',
    '  }',
    '  gl_FragColor = vec4(uDot, dotMask * (0.55 + amp * 2.40));',
    '}'
  ].join('\n');

  var canvas = document.createElement('canvas');
  canvas.className = 'wave-bg';
  canvas.setAttribute('aria-hidden', 'true');
  host.parentNode.insertBefore(canvas, host.nextSibling);

  var gl = null;
  try {
    gl = canvas.getContext('webgl', { alpha: true, antialias: false, depth: false, stencil: false, powerPreference: 'low-power' });
  } catch (e) { gl = null; }

  function drop() {
    if (canvas.parentNode) { canvas.parentNode.removeChild(canvas); }
  }
  if (!gl) { drop(); return; }

  function compile(type, src) {
    var s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) { gl.deleteShader(s); return null; }
    return s;
  }

  var vs = compile(gl.VERTEX_SHADER, VERT);
  var fs = compile(gl.FRAGMENT_SHADER, FRAG);
  if (!vs || !fs) { drop(); return; }

  var prog = gl.createProgram();
  gl.attachShader(prog, vs);
  gl.attachShader(prog, fs);
  gl.linkProgram(prog);
  if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) { drop(); return; }
  gl.useProgram(prog);

  // One oversized triangle covers the viewport with no index buffer and no seam down the diagonal
  // that two triangles would have to share.
  // 一个超大的三角形盖住整个视口：不需要索引缓冲，也没有两个三角形必须共享的那条对角线接缝。
  var buf = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, buf);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  var aPos = gl.getAttribLocation(prog, 'aPos');
  gl.enableVertexAttribArray(aPos);
  gl.vertexAttribPointer(aPos, 2, gl.FLOAT, false, 0, 0);
  gl.enable(gl.BLEND);
  gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);

  var uRes = gl.getUniformLocation(prog, 'uRes');
  var uDpr = gl.getUniformLocation(prog, 'uDpr');
  var uDot = gl.getUniformLocation(prog, 'uDot');
  var uPitch = gl.getUniformLocation(prog, 'uPitch');
  var uTime = gl.getUniformLocation(prog, 'uTime');
  var uWave = gl.getUniformLocation(prog, 'uWave');
  var uCount = gl.getUniformLocation(prog, 'uCount');
  gl.uniform1f(uPitch, 20.0);

  // Read the dot colour from the same custom property the static grid uses, so the two cannot drift
  // apart when the theme changes.
  // 点色取自静态网格用的同一个自定义属性，两者不会在主题切换时各走各的。
  var dotRGB = [0.82, 0.82, 0.82];
  function readDot() {
    var raw = '';
    try { raw = getComputedStyle(body).getPropertyValue('--dot').trim(); } catch (e) { raw = ''; }
    var hex6 = /^#([0-9a-f]{6})$/i.exec(raw);
    if (hex6) {
      var n = parseInt(hex6[1], 16);
      dotRGB = [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
      return;
    }
    var hex3 = /^#([0-9a-f]{3})$/i.exec(raw);
    if (hex3) {
      var h = hex3[1];
      dotRGB = [parseInt(h[0] + h[0], 16) / 255, parseInt(h[1] + h[1], 16) / 255, parseInt(h[2] + h[2], 16) / 255];
      return;
    }
    var rgb = /rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/.exec(raw);
    if (rgb) { dotRGB = [+rgb[1] / 255, +rgb[2] / 255, +rgb[3] / 255]; }
  }
  readDot();

  var dpr = Math.min(window.devicePixelRatio || 1, 2);
  var W = 0, H = 0;
  function resize() {
    // Measure the element, not the window.
    //
    // window.innerWidth includes the classic scrollbar's 15px, while a position:fixed box spans
    // only the viewport it can paint in (1440 vs 1425 on a 1440px window). Sizing the backing store
    // from innerWidth left the shader one strip too wide, and every wave seeded by Math.random()*W
    // could land in that invisible strip. clientWidth is the box the canvas actually occupies.
    //
    // 量元素，不量窗口。
    //
    // window.innerWidth 包含经典滚动条那 15px，而 position:fixed 的盒子只覆盖它能画到的那块视口
    // （1440px 窗口上是 1425 对 1440）。按 innerWidth 定尺寸会让着色器宽出一条不可见的窄带，
    // 而每一条用 Math.random()*W 播下的波都可能落进那条带里。clientWidth 才是画布真正占的框。
    var w = canvas.clientWidth || window.innerWidth;
    var h = canvas.clientHeight || window.innerHeight;
    if (w <= 0 || h <= 0) { return; }
    var cw = Math.round(w * dpr), ch = Math.round(h * dpr);
    if (canvas.width === cw && canvas.height === ch) { return; }
    canvas.width = cw;
    canvas.height = ch;
    W = w; H = h;
    gl.viewport(0, 0, cw, ch);
    gl.uniform2f(uRes, W, H);
    gl.uniform1f(uDpr, dpr);
  }
  resize();

  var MAX = 6;
  var waves = [];
  var arr = new Float32Array(MAX * 3);
  var clock = 0;

  function spawn(x, y) {
    waves.push({ x: x, y: y, born: clock });
    while (waves.length > MAX) { waves.shift(); }
  }

  // The heartbeat. Intervals are irregular on purpose: a fixed period reads as a metronome, and a
  // metronome is the one thing a "living" surface must not sound like.
  //
  // 脉搏。间隔刻意不等长：固定周期读起来像节拍器，而节拍器恰恰是一片「活着的」表面最不该像的东西。
  var timer = null;
  function beat() {
    timer = window.setTimeout(function () {
      if (!document.hidden && W > 0 && H > 0) {
        spawn(Math.random() * W, Math.random() * H);
      }
      beat();
    }, 380 + Math.random() * 900);
  }
  beat();
  spawn(W * 0.32, H * 0.34);
  spawn(W * 0.68, H * 0.58);

  var raf = 0;
  var t0 = performance.now();

  function frame(ts) {
    clock = (ts - t0) / 1000;
    while (waves.length && clock - waves[0].born > 2.4) { waves.shift(); }
    arr.fill(0);
    for (var i = 0; i < waves.length; i++) {
      arr[i * 3] = waves[i].x;
      arr[i * 3 + 1] = waves[i].y;
      arr[i * 3 + 2] = waves[i].born;
    }
    gl.uniform3fv(uWave, arr);
    gl.uniform1i(uCount, waves.length);
    gl.uniform1f(uTime, clock);
    gl.uniform3fv(uDot, dotRGB);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    raf = window.requestAnimationFrame(frame);
  }
  raf = window.requestAnimationFrame(frame);

  // Stop drawing when the tab is hidden, and resume on a clock that did not jump: carrying the
  // wall-clock gap into uTime would fire every wave at once on return.
  //
  // 标签页隐藏时停止绘制，并在恢复时用没有跳变的时钟：把挂起期间的真实时长带进 uTime，
  // 会让所有波在回来的那一瞬间同时炸开。
  document.addEventListener('visibilitychange', function () {
    if (document.hidden) {
      window.cancelAnimationFrame(raf);
      raf = 0;
    } else if (!raf) {
      t0 = performance.now() - clock * 1000;
      raf = window.requestAnimationFrame(frame);
    }
  });

  // Re-read the dot colour whenever the theme could have changed. Three routes reach dark mode
  // (explicit dark, explicit light, follow-system), so both the class and the media query matter.
  // 主题可能变化时重读点色。到达暗色有三条路（显式 dark、显式 light、跟随系统），
  // 因此 class 与媒体查询都要看。
  try {
    new MutationObserver(readDot).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  } catch (e) {}
  if (window.matchMedia) {
    var mq = window.matchMedia('(prefers-color-scheme: dark)');
    if (mq.addEventListener) { mq.addEventListener('change', readDot); }
    else if (mq.addListener) { mq.addListener(readDot); }
  }

  var resizeTimer = null;
  window.addEventListener('resize', function () {
    if (resizeTimer) { window.clearTimeout(resizeTimer); }
    resizeTimer = window.setTimeout(resize, 120);
  });

  // Crossing below the breakpoint hands the background back to the static grid.
  // 跨到断点以下时把背景交还给静态网格。
  function onBreak() {
    if (desk.matches) { return; }
    window.cancelAnimationFrame(raf);
    raf = 0;
    if (timer) { window.clearTimeout(timer); timer = null; }
    drop();
  }
  if (desk.addEventListener) { desk.addEventListener('change', onBreak); }
  else if (desk.addListener) { desk.addListener(onBreak); }
})();
`
