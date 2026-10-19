package routers

// Generated asset fingerprint. Regenerate after replacing frontend/static/js/d3-graph.min.js;
// the helper at the bottom of this comment explains why it exists.
//
// The static route sends max-age=31536000 for everything under /static, which is correct only
// if the URL changes whenever the bytes change. d3-graph.min.js is replaced in place, so
// without a fingerprint every reader keeps the copy their browser cached on first visit —
// measured consequence: a fixed graph still behaved like the broken one for a full year.
//
// 生成的资源指纹。替换 frontend/static/js/d3-graph.min.js 之后需要重新生成。
//
// /static 下的一切都带 max-age=31536000，这只在「字节一变 URL 就变」时才是正确的。
// d3-graph.min.js 是原地覆盖的，所以没有指纹时，每个读者都会一直用首次访问时缓存的那一份——
// 实测后果：一个已经修好的图谱，在读者那里整整一年都表现为坏的那个样子。
const d3GraphAssetVersion = "3a0f47990817"
