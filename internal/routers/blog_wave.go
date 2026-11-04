package routers

// The dot grid becomes a lit surface.
//
// This is a port of the PatternWaves reference the owner supplied: a field pass computes, per grid
// cell, a height from a layered silk wave, derives a normal from that height's gradient, and shades
// it with a diffuse term plus a tight specular highlight. A second pass then draws each dot sized and
// faded by the shaded level. The visible consequence is that the grid stops being flat texture and
// starts behaving like a fabric being breathed on — bright ridges travel across it and the dots along
// a ridge swell.
//
// Three things are deliberately different from the reference:
//
//   - No ogl. The reference depends on a WebGL wrapper for render targets and program setup; this is
//     one self-contained script with the same three passes. A wrapper here would be a second runtime
//     to keep alive for about forty lines of state.
//   - The dot colour comes from the stylesheet's --dot, not from a prop. The static grid and the
//     shader must be the same colour or the theme switch makes one of them wrong, and the stylesheet
//     is where the theme already lives.
//   - Ripples are self-touching as well as pointer-driven. A surface that only reacts to a pointer is
//     motionless whenever nobody is pointing at it, which is most of the time.
//
// 点阵变成一片被打光的表面。
//
// 这是站主给的 PatternWaves 参考实现的移植：field 这一趟按格子算出 silk 波形的高度，由高度梯度
// 求出法线，再用漫反射加一道很紧的高光上色；第二趟按着色后的亮度决定每个点的大小与浓淡。
// 可见的变化是网格不再是平的纹理，而开始像一块被呼吸吹动的织物——亮的脊线会扫过它，
// 脊线上的点随之涨大。
//
// 与参考实现有三处刻意不同：
//
//   - 不用 ogl。参考实现依赖一个 WebGL 封装来管理渲染目标与程序装配；这里是同一个三趟管线、
//     一个自包含脚本。为了约四十行状态而引入第二个运行时并不划算。
//   - 点色取自样式表的 --dot，而不是从属性传入。静态网格与着色器必须是同一种颜色，
//     否则主题一换就有一个是错的，而主题本来就住在样式表里。
//   - 涟漪既由指针驱动，也自己发生。一片只在有人指向时才动的表面，在没人指的时候就是静止的，
//     而那是绝大部分时间。
const blogWaveJS = `
(function () {
  'use strict';

  var body = document.body;
  if (!body) { return; }
  if (!body.classList.contains('is-home') && !body.classList.contains('is-page')) { return; }

  var host = document.querySelector('.backdrop');
  if (!host) { return; }

  var desk = window.matchMedia('(min-width: 861px)');
  var calm = window.matchMedia('(prefers-reduced-motion: reduce)');
  if (!desk.matches || calm.matches) { return; }

  var canvas = document.createElement('canvas');
  canvas.className = 'wave-bg';
  canvas.setAttribute('aria-hidden', 'true');
  host.parentNode.insertBefore(canvas, host.nextSibling);

  var gl = null;
  try {
    gl = canvas.getContext('webgl2', {
      alpha: true, antialias: false, depth: false, stencil: false,
      premultipliedAlpha: true, powerPreference: 'low-power'
    });
  } catch (e) { gl = null; }

  function drop() { if (canvas.parentNode) { canvas.parentNode.removeChild(canvas); } }
  if (!gl) { drop(); return; }

  // ── shaders ─────────────────────────────────────────────────────────
  var VERT = [
    '#version 300 es',
    'in vec2 position;',
    'void main() { gl_Position = vec4(position, 0.0, 1.0); }'
  ].join('\n');

  var FIELD = [
    '#version 300 es',
    'precision highp float;',
    'precision highp int;',
    'uniform vec2 uSize;',
    'uniform float uDpr;',
    'uniform vec2 uOrigin;',
    'uniform vec2 uPitch;',
    'uniform float uTime;',
    'uniform float uUnit;',
    'uniform vec2 uHeading;',
    'uniform float uAmp;',
    'uniform float uDepth;',
    'uniform vec3 uLight;',
    'uniform float uShine;',
    'uniform float uContrast;',
    'uniform float uOpacity;',
    'uniform int uFade;',
    'uniform float uFadeSize;',
    'uniform float uAppear;',
    'uniform sampler2D tRipple;',
    'uniform float uRipple;',
    'out vec4 fragColor;',
    'const float FOLDS = 5.5;',
    'uvec3 scramble(uvec3 v) {',
    '  v = v * 1664525u + 1013904223u;',
    '  v.x += v.y * v.z; v.y += v.z * v.x; v.z += v.x * v.y;',
    '  v ^= v >> 16u;',
    '  v.x += v.y * v.z; v.y += v.z * v.x; v.z += v.x * v.y;',
    '  return v;',
    '}',
    'vec3 lattice(vec3 corner) {',
    '  uvec3 h = scramble(uvec3(ivec3(corner) + 4096));',
    '  return vec3(h & 65535u) / 32767.5 - 1.0;',
    '}',
    'float gradientNoise(vec3 p) {',
    '  vec3 i = floor(p);',
    '  vec3 f = p - i;',
    '  vec3 u = f * f * f * (f * (f * 6.0 - 15.0) + 10.0);',
    '  float n000 = dot(lattice(i), f);',
    '  float n100 = dot(lattice(i + vec3(1.0, 0.0, 0.0)), f - vec3(1.0, 0.0, 0.0));',
    '  float n010 = dot(lattice(i + vec3(0.0, 1.0, 0.0)), f - vec3(0.0, 1.0, 0.0));',
    '  float n110 = dot(lattice(i + vec3(1.0, 1.0, 0.0)), f - vec3(1.0, 1.0, 0.0));',
    '  float n001 = dot(lattice(i + vec3(0.0, 0.0, 1.0)), f - vec3(0.0, 0.0, 1.0));',
    '  float n101 = dot(lattice(i + vec3(1.0, 0.0, 1.0)), f - vec3(1.0, 0.0, 1.0));',
    '  float n011 = dot(lattice(i + vec3(0.0, 1.0, 1.0)), f - vec3(0.0, 1.0, 1.0));',
    '  float n111 = dot(lattice(i + vec3(1.0, 1.0, 1.0)), f - vec3(1.0, 1.0, 1.0));',
    '  return mix(mix(mix(n000, n100, u.x), mix(n010, n110, u.x), u.y),',
    '             mix(mix(n001, n101, u.x), mix(n011, n111, u.x), u.y), u.z);',
    '}',
    // One silk fold: a sine train along the heading, bent by noise across it, with a slow swell so
    // the ridges do not arrive at a metronome's pace.
    // 一道丝绸褶皱：沿朝向的正弦列，被横向的噪声弯曲，再叠一层很慢的涌浪，
    // 使脊线不会以节拍器的节奏到达。
    'float surface(vec2 p, float t) {',
    '  vec2 side = vec2(-uHeading.y, uHeading.x);',
    '  float u = dot(p, uHeading);',
    '  float v = dot(p, side);',
    '  float bend = gradientNoise(vec3(v * 0.85, u * 0.3, t * 0.05)) * 1.7 + 0.4 * sin(v * 1.6 + t * 0.2);',
    '  float phase = u * FOLDS + bend - t * 0.45;',
    '  float swell = 0.6 + 0.4 * gradientNoise(vec3(u * 0.55 + 3.0, v * 0.45, t * 0.04));',
    '  float fold = sin(phase) + 0.32 * sin(2.0 * phase + 1.3);',
    '  float grain = 0.16 * sin(u * FOLDS * 2.5 + bend * 1.9 - t * 0.9 + 2.1);',
    '  return (fold + grain) * swell;',
    '}',
    'float heightAt(vec2 css) {',
    '  float h = surface((css - 0.5 * uSize) / uUnit, uTime) * uAmp;',
    '  if (uRipple > 0.0) { h += texture(tRipple, css / uSize).r * uRipple; }',
    '  return h;',
    '}',
    'void main() {',
    '  vec2 cell = floor(gl_FragCoord.xy);',
    '  vec2 center = uOrigin + (cell + 0.5) * uPitch;',
    '  vec2 css = center / uDpr;',
    '  vec2 uv = css / uSize;',
    '  float e = max(uPitch.y / uDpr, 4.0);',
    '  float h = heightAt(css);',
    '  float hx = (heightAt(css + vec2(e, 0.0)) - heightAt(css - vec2(e, 0.0))) / (2.0 * e);',
    '  float hy = (heightAt(css + vec2(0.0, e)) - heightAt(css - vec2(0.0, e))) / (2.0 * e);',
    '  vec2 grad = vec2(hx, hy) * uUnit * uDepth * 0.4;',
    '  vec3 n = normalize(vec3(-grad, 1.0));',
    '  float diffuse = clamp(dot(n, uLight), 0.0, 1.0);',
    '  vec3 halfway = normalize(uLight + vec3(0.0, 0.0, 1.0));',
    '  float spec = pow(clamp(dot(n, halfway), 0.0, 1.0), 160.0) * uShine * 1.15;',
    // A second, far wider highlight. The tight one above fires only where a cell's normal points
    // almost exactly at the halfway vector, which at a 5px pitch is a handful of cells per ridge —
    // measured, it contributed nothing a reader could see. This one is a broad sheen along the
    // whole ridge, which is what reads as light travelling across the surface.
    //
    // 第二道宽得多的高光。上面那道只在格子的法线几乎正对半角向量时才亮，
    // 在 5px 间距下一条脊线上只有寥寥几格——实测它没有贡献任何读者看得见的东西。
    // 这一道是沿整条脊线的柔光，那才是「光在表面上走」的观感来源。
    '  float sheen = pow(clamp(dot(n, halfway), 0.0, 1.0), 8.0) * uShine * 0.42;',
    '  float hollow = 0.7 + 0.3 * clamp(h * 0.5 + 0.5, 0.0, 1.0);',
    '  float tone = clamp(diffuse * hollow * 0.78 + spec + sheen, 0.0, 1.0);',
    '  tone = clamp((tone - 0.42) * uContrast + 0.42, 0.0, 1.0);',
    '  float level = pow(tone, 2.2);',
    '  float fade = 1.0;',
    '  vec2 c = uv * 2.0 - 1.0;',
    '  if (uFade == 1) {',
    '    fade = 1.0 - smoothstep(1.0 - uFadeSize, 1.18, length(c));',
    '  } else if (uFade == 2) {',
    '    fade = smoothstep(0.0, uFadeSize, uv.y);',
    '  }',
    '  float reach = length(css - 0.5 * uSize) / max(0.5 * length(uSize), 1.0);',
    '  float appear = smoothstep(reach - 0.05, reach + 0.3, uAppear * 1.35);',
    '  float alpha = uOpacity * fade * appear;',
    '  fragColor = vec4(level, alpha, clamp(spec * 1.6, 0.0, 1.0), clamp(h * uDepth * 0.42, -0.48, 0.48) + 0.5);',
    '}'
  ].join('\n');

  var RIPPLE = [
    '#version 300 es',
    'precision highp float;',
    'uniform sampler2D tState;',
    'uniform vec2 uTexel;',
    'uniform vec2 uSize;',
    'uniform vec2 uFrom;',
    'uniform vec2 uTo;',
    'uniform float uRadius;',
    'uniform float uImpulse;',
    'uniform float uDamping;',
    'out vec4 fragColor;',
    'void main() {',
    '  vec2 uv = gl_FragCoord.xy * uTexel;',
    '  vec2 state = texture(tState, uv).rg;',
    '  float left = texture(tState, uv - vec2(uTexel.x, 0.0)).r;',
    '  float right = texture(tState, uv + vec2(uTexel.x, 0.0)).r;',
    '  float below = texture(tState, uv - vec2(0.0, uTexel.y)).r;',
    '  float above = texture(tState, uv + vec2(0.0, uTexel.y)).r;',
    '  float next = ((left + right + below + above) * 0.5 - state.g) * uDamping;',
    '  vec2 p = uv * uSize;',
    '  vec2 segment = uTo - uFrom;',
    '  float along = clamp(dot(p - uFrom, segment) / max(dot(segment, segment), 1e-4), 0.0, 1.0);',
    '  float d = length(p - uFrom - segment * along) / uRadius;',
    '  next -= uImpulse * exp(-d * d * 2.0);',
    '  fragColor = vec4(next, state.r, 0.0, 1.0);',
    '}'
  ].join('\n');

  var MARK = [
    '#version 300 es',
    'precision highp float;',
    'uniform sampler2D tField;',
    'uniform vec2 uOrigin;',
    'uniform vec2 uPitch;',
    'uniform vec2 uGrid;',
    'uniform float uMarkSize;',
    'uniform vec3 uColor;',
    'uniform vec3 uFore;',
    'out vec4 fragColor;',
    'void main() {',
    '  vec2 rel = gl_FragCoord.xy - uOrigin;',
    '  float cx = floor(rel.x / uPitch.x);',
    '  float row = floor(rel.y / uPitch.y);',
    '  vec4 ink = vec4(0.0);',
    '  if (cx >= 0.0 && cx < uGrid.x) {',
    // The vertical offset of a dot depends on the height of the cell, so neighbouring rows are
    // sampled too — otherwise a ridge would clip the dots just above and below it.
    // 一个点的纵向偏移取决于该格的高度，因此也要采样相邻行；
    // 否则一道脊线会把紧邻它上下两侧的点裁掉。
    '    for (int k = -1; k <= 1; k++) {',
    '      float cy = row + float(k);',
    '      if (cy < 0.0 || cy >= uGrid.y) continue;',
    '      vec4 f = texelFetch(tField, ivec2(int(cx), int(cy)), 0);',
    '      if (f.g < 0.002) continue;',
    '      vec2 center = (vec2(cx, cy) + 0.5) * uPitch + vec2(0.0, (f.a - 0.5) * uPitch.y);',
    '      float span = uPitch.y * uMarkSize;',
    '      float area = 0.62 + 0.38 * clamp(sqrt(f.r), 0.0, 1.0);',
    '      float alpha = clamp(0.5 - (length(rel - center) - 0.5 * span * area), 0.0, 1.0);',
    '      alpha *= f.g;',
    '      if (alpha > ink.a) {',
    '        vec3 tint = mix(uColor, uFore, clamp(f.r * 0.36, 0.0, 1.0));',
    '        ink = vec4(tint * alpha, alpha);',
    '      }',
    '    }',
    '  }',
    '  fragColor = ink;',
    '}'
  ].join('\n');

  function compile(type, src) {
    var s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
      gl.deleteShader(s);
      return null;
    }
    return s;
  }

  function program(fragSrc) {
    var vs = compile(gl.VERTEX_SHADER, VERT);
    var fs = compile(gl.FRAGMENT_SHADER, fragSrc);
    if (!vs || !fs) { return null; }
    var p = gl.createProgram();
    gl.attachShader(p, vs);
    gl.attachShader(p, fs);
    gl.bindAttribLocation(p, 0, 'position');
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) { return null; }
    return p;
  }

  var fieldProg = program(FIELD);
  var rippleProg = program(RIPPLE);
  var markProg = program(MARK);
  if (!fieldProg || !markProg) { drop(); return; }

  // ── geometry ────────────────────────────────────────────────────────
  var vao = gl.createVertexArray();
  gl.bindVertexArray(vao);
  var vbo = gl.createBuffer();
  gl.bindBuffer(gl.ARRAY_BUFFER, vbo);
  gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW);
  gl.enableVertexAttribArray(0);
  gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0);
  gl.bindVertexArray(null);

  var uField = {};
  var uRipple = {};
  var uMark = {};
  ['uSize', 'uDpr', 'uOrigin', 'uPitch', 'uTime', 'uUnit', 'uHeading', 'uAmp',
   'uDepth', 'uLight', 'uShine', 'uContrast', 'uOpacity', 'uFade', 'uFadeSize',
   'uAppear', 'tRipple', 'uRipple'].forEach(function (k) { uField[k] = gl.getUniformLocation(fieldProg, k); });
  if (rippleProg) {
    ['tState', 'uTexel', 'uSize', 'uFrom', 'uTo', 'uRadius', 'uImpulse', 'uDamping']
      .forEach(function (k) { uRipple[k] = gl.getUniformLocation(rippleProg, k); });
  }
  ['tField', 'uOrigin', 'uPitch', 'uGrid', 'uMarkSize', 'uColor', 'uFore']
    .forEach(function (k) { uMark[k] = gl.getUniformLocation(markProg, k); });

  // ── render targets ──────────────────────────────────────────────────
  var floatOk = !!(gl.getExtension('EXT_color_buffer_float') || gl.getExtension('EXT_color_buffer_half_float'));

  function makeTarget(w, h, half) {
    var tex = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, tex);
    if (half) {
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RG16F, w, h, 0, gl.RG, gl.HALF_FLOAT, null);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    } else {
      gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, w, h, 0, gl.RGBA, gl.UNSIGNED_BYTE, null);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    }
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    var fb = gl.createFramebuffer();
    gl.bindFramebuffer(gl.FRAMEBUFFER, fb);
    gl.framebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, tex, 0);
    var ok = gl.checkFramebufferStatus(gl.FRAMEBUFFER) === gl.FRAMEBUFFER_COMPLETE;
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    return ok ? { tex: tex, fb: fb, w: w, h: h } : null;
  }

  function freeTarget(t) {
    if (!t) { return; }
    gl.deleteFramebuffer(t.fb);
    gl.deleteTexture(t.tex);
  }

  // ── palette ─────────────────────────────────────────────────────────
  // Both colours come from the stylesheet. The dot colour is what the static grid uses; the
  // foreground is where a wave crest is allowed to brighten to. Without that second one the grid is
  // invisible on a dark theme: --dot is #333 there, and #333 dots lit to #333 are still #333.
  //
  // 两种颜色都取自样式表。点色就是静态网格用的那个；前景色是波峰允许提亮到的上限。
  // 少了后者，暗色主题下整片网格等于不存在：那里 --dot 是 #333，而把 #333 的点「打亮」还是 #333。
  var dotRGB = [0.82, 0.82, 0.82];
  var foreRGB = [0.35, 0.34, 0.31];

  function parseVar(name, fallback) {
    var raw = '';
    try { raw = getComputedStyle(body).getPropertyValue(name).trim(); } catch (e) { raw = ''; }
    var h6 = /^#([0-9a-f]{6})$/i.exec(raw);
    if (h6) {
      var n = parseInt(h6[1], 16);
      return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
    }
    var h3 = /^#([0-9a-f]{3})$/i.exec(raw);
    if (h3) {
      var s = h3[1];
      return [parseInt(s[0] + s[0], 16) / 255, parseInt(s[1] + s[1], 16) / 255, parseInt(s[2] + s[2], 16) / 255];
    }
    var m = /rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/.exec(raw);
    if (m) { return [+m[1] / 255, +m[2] / 255, +m[3] / 255]; }
    return fallback;
  }

  function readPalette() {
    dotRGB = parseVar('--dot', dotRGB);
    foreRGB = parseVar('--foreground', foreRGB);
  }
  readPalette();

  // ── tuning (the owner's numbers) ─────────────────────────────────────
  var SPACING = 5;
  var MARK_SIZE = 0.62;
  var DEPTH = 0.9;
  var LIGHT = 1;
  var SHINE = 1;
  var CONTRAST = 1.5;
  var SPEED = 1.4;
  var SCALE = 2.0;
  var DIRECTION = 0;
  var FADE_MODE = 2;
  var FADE_SIZE = 0.34;
  var WAVE_UNIT = 520;
  var PIXEL_BUDGET = 4.5e6;
  var INTRO_SECONDS = 2;
  var RIPPLE_CELL = 8;
  var RIPPLE_RATE = 60;

  function clamp(v, lo, hi) { return Math.min(Math.max(v, lo), hi); }

  var W = 1, H = 1, dpr = 1;
  var field = null;
  var rippleRead = null, rippleWrite = null;
  var rippleClock = 0, rippleUntil = 0, rippleAmp = 0;
  var time = 0, introClock = 0;

  function buildTargets() {
    var pitch = Math.max(4, Math.round(SPACING * dpr));
    var cols = Math.min(4096, Math.ceil(gl.drawingBufferWidth / pitch) + 1);
    var rows = Math.min(4096, Math.ceil(gl.drawingBufferHeight / pitch) + 2);
    if (!field || field.w !== cols || field.h !== rows) {
      freeTarget(field);
      field = makeTarget(cols, rows, false);
    }
    if (floatOk && rippleProg) {
      var rw = clamp(Math.ceil(W / RIPPLE_CELL), 4, 512);
      var rh = clamp(Math.ceil(H / RIPPLE_CELL), 4, 512);
      if (!rippleRead || rippleRead.w !== rw || rippleRead.h !== rh) {
        freeTarget(rippleRead);
        freeTarget(rippleWrite);
        rippleRead = makeTarget(rw, rh, true);
        rippleWrite = makeTarget(rw, rh, true);
      }
    }
  }

  function resize() {
    var cw = canvas.clientWidth || window.innerWidth;
    var ch = canvas.clientHeight || window.innerHeight;
    if (cw <= 0 || ch <= 0) { return; }
    W = cw; H = ch;
    dpr = Math.min(window.devicePixelRatio || 1, 2, Math.sqrt(PIXEL_BUDGET / (W * H)));
    var w = Math.round(W * dpr), h = Math.round(H * dpr);
    if (canvas.width === w && canvas.height === h) { return; }
    canvas.width = w;
    canvas.height = h;
    buildTargets();
  }

  // ── pointer + self ripples ──────────────────────────────────────────
  var pointer = { x: 0, y: 0, lastX: 0, lastY: 0, inside: false, placed: false, burst: 0 };

  function locate(e) {
    var r = canvas.getBoundingClientRect();
    var x = e.clientX - r.left;
    var y = e.clientY - r.top;
    return { x: x, y: y, inside: x >= 0 && y >= 0 && x <= r.width && y <= r.height };
  }

  function onMove(e) {
    var p = locate(e);
    pointer.x = p.x; pointer.y = p.y;
    if (p.inside !== pointer.inside) { pointer.placed = false; }
    pointer.inside = p.inside;
  }
  function onDown(e) {
    var p = locate(e);
    if (!p.inside) { return; }
    pointer.x = p.x; pointer.y = p.y;
    if (!pointer.inside) { pointer.lastX = p.x; pointer.lastY = p.y; }
    pointer.burst = 1.2;
    pointer.inside = true;
  }
  function onLeave() { pointer.inside = false; pointer.placed = false; }

  window.addEventListener('pointermove', onMove, { passive: true });
  window.addEventListener('pointerdown', onDown, { passive: true });
  window.addEventListener('pointerup', function (e) { if (e.pointerType === 'touch') { onLeave(); } }, { passive: true });
  window.addEventListener('blur', onLeave);

  // Self-touch: a ripple nobody asked for, at an irregular interval.
  // 自触：一次没人要求的涟漪，间隔不等长。
  var beatTimer = null;
  function beat() {
    beatTimer = window.setTimeout(function () {
      if (!document.hidden && rippleRead) {
        rippleUntil = performance.now() + 2600;
        var x = Math.random() * W, y = Math.random() * H;
        pointer.lastX = x; pointer.lastY = y;
        pointer.x = x; pointer.y = y;
        pointer.burst = 0.55;
        pointer.placed = true;
        kick();
      }
      beat();
    }, 1500 + Math.random() * 2600);
  }

  var raf = 0;
  var last = 0;

  function kick() { if (!raf) { last = performance.now(); raf = requestAnimationFrame(frame); } }

  function stepRipple(dt) {
    if (!rippleRead || !rippleWrite) { return; }
    rippleClock = Math.min(rippleClock + dt * RIPPLE_RATE, 4);
    var moved = Math.hypot(pointer.x - pointer.lastX, pointer.y - pointer.lastY);
    var impulse = (pointer.inside ? Math.min(moved / 14, 1) * 0.35 : 0) + pointer.burst;
    pointer.burst = 0;
    gl.useProgram(rippleProg);
    gl.bindVertexArray(vao);
    gl.uniform2f(uRipple.uTexel, 1 / rippleRead.w, 1 / rippleRead.h);
    gl.uniform2f(uRipple.uSize, W, H);
    gl.uniform2f(uRipple.uFrom, pointer.lastX, H - pointer.lastY);
    gl.uniform2f(uRipple.uTo, pointer.x, H - pointer.y);
    gl.uniform1f(uRipple.uRadius, 20);
    gl.uniform1f(uRipple.uDamping, 0.975);
    while (rippleClock >= 1) {
      rippleClock -= 1;
      gl.bindFramebuffer(gl.FRAMEBUFFER, rippleWrite.fb);
      gl.viewport(0, 0, rippleWrite.w, rippleWrite.h);
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, rippleRead.tex);
      gl.uniform1i(uRipple.tState, 0);
      gl.uniform1f(uRipple.uImpulse, impulse);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      var t = rippleRead; rippleRead = rippleWrite; rippleWrite = t;
      impulse = 0;
    }
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    pointer.lastX = pointer.x;
    pointer.lastY = pointer.y;
  }

  function frame(now) {
    raf = 0;
    if (!field) { return; }
    var dt = Math.min(0.05, Math.max(1 / 240, (now - last) / 1000));
    last = now;
    time += dt * SPEED;
    introClock = Math.min(1, introClock + dt / INTRO_SECONDS);
    var appear = 1 - Math.pow(1 - clamp(introClock / 0.75, 0, 1), 3);
    var rise = clamp((introClock - 0.1) / 0.9, 0, 1);
    var amp = rise * rise * (3 - 2 * rise);

    // Ripple amplitude eases instead of switching, so the surface settles rather than snapping
    // flat the instant the last ripple expires.
    // 涟漪幅度是渐变的，不是开关：最后一道涟漪结束之后表面慢慢平息，而不是一下子变平。
    var wantRipple = !!rippleRead && (pointer.inside || pointer.burst > 0 || now < rippleUntil);
    if (wantRipple) { stepRipple(dt); }
    var targetAmp = wantRipple ? 0.32 : 0;
    rippleAmp += (targetAmp - rippleAmp) * Math.min(1, dt * 4);
    if (rippleAmp < 0.005) { rippleAmp = 0; }

    var heading = (DIRECTION * Math.PI) / 180;
    var lightAngle = ((DIRECTION + 180 + clamp(LIGHT, -90, 90)) * Math.PI) / 180;

    gl.useProgram(fieldProg);
    gl.bindVertexArray(vao);
    gl.bindFramebuffer(gl.FRAMEBUFFER, field.fb);
    gl.viewport(0, 0, field.w, field.h);
    gl.uniform2f(uField.uSize, W, H);
    gl.uniform1f(uField.uDpr, dpr);
    gl.uniform2f(uField.uOrigin, 0, 0);
    gl.uniform2f(uField.uPitch, Math.max(4, Math.round(SPACING * dpr)), Math.max(4, Math.round(SPACING * dpr)));
    gl.uniform1f(uField.uTime, time);
    gl.uniform1f(uField.uUnit, WAVE_UNIT * SCALE);
    gl.uniform2f(uField.uHeading, Math.cos(heading), Math.sin(heading));
    gl.uniform1f(uField.uAmp, amp);
    gl.uniform1f(uField.uDepth, DEPTH);
    gl.uniform3f(uField.uLight, Math.cos(lightAngle) * 0.78, Math.sin(lightAngle) * 0.78, 0.62);
    gl.uniform1f(uField.uShine, SHINE);
    gl.uniform1f(uField.uContrast, CONTRAST);
    gl.uniform1f(uField.uOpacity, 1);
    gl.uniform1i(uField.uFade, FADE_MODE);
    gl.uniform1f(uField.uFadeSize, FADE_SIZE);
    gl.uniform1f(uField.uAppear, appear);
    gl.uniform1f(uField.uRipple, rippleAmp);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, rippleRead ? rippleRead.tex : null);
    gl.uniform1i(uField.tRipple, 0);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    gl.useProgram(markProg);
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    gl.viewport(0, 0, canvas.width, canvas.height);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, field.tex);
    gl.uniform1i(uMark.tField, 0);
    gl.uniform2f(uMark.uOrigin, 0, 0);
    gl.uniform2f(uMark.uPitch, Math.max(4, Math.round(SPACING * dpr)), Math.max(4, Math.round(SPACING * dpr)));
    gl.uniform2f(uMark.uGrid, field.w, field.h);
    gl.uniform1f(uMark.uMarkSize, MARK_SIZE);
    gl.uniform3fv(uMark.uColor, dotRGB);
    gl.uniform3fv(uMark.uFore, foreRGB);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
    gl.disable(gl.BLEND);
    gl.bindVertexArray(null);

    kick();
  }

  resize();
  beat();

  var resizeTimer = null;
  window.addEventListener('resize', function () {
    if (resizeTimer) { window.clearTimeout(resizeTimer); }
    resizeTimer = window.setTimeout(function () { resize(); kick(); }, 120);
  });

  document.addEventListener('visibilitychange', function () {
    if (document.hidden) {
      if (raf) { cancelAnimationFrame(raf); raf = 0; }
    } else {
      kick();
    }
  });

  try {
    new MutationObserver(readPalette).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  } catch (e) {}
  if (window.matchMedia) {
    var mq = window.matchMedia('(prefers-color-scheme: dark)');
    if (mq.addEventListener) { mq.addEventListener('change', readPalette); }
    else if (mq.addListener) { mq.addListener(readPalette); }
  }

  function onBreak() {
    if (desk.matches) { return; }
    if (raf) { cancelAnimationFrame(raf); raf = 0; }
    if (beatTimer) { window.clearTimeout(beatTimer); beatTimer = null; }
    freeTarget(field); field = null;
    freeTarget(rippleRead); rippleRead = null;
    freeTarget(rippleWrite); rippleWrite = null;
    drop();
  }
  if (desk.addEventListener) { desk.addEventListener('change', onBreak); }
  else if (desk.addListener) { desk.addListener(onBreak); }

  kick();
})();
`
