package service

import "errors"

// Retrieval concurrency gate for the blog Q&A entry.
//
// One question has two phases with opposite resource profiles. Retrieval — embedding the
// question, probing Bleve, scoring vectors, calling the cross-encoder — takes 2–3 seconds and
// is pure spend: every question bills an embedding round trip and a rerank round trip, and the
// money leaves whether or not anyone reads the answer. Generation takes 10–30 seconds and is
// nearly free locally: it holds one upstream connection and no CPU.
//
// So the two phases need different limits. Retrieval gets a small global gate; generation gets
// a per-visitor lock (enforced in the router, where the client identity lives) and no global
// cap at all.
//
// The reason the gate must be *narrow* is worth stating plainly, because the obvious
// implementation is the wrong one: a single global semaphore wrapping the whole request would
// be held for the entire 10–30-second generation, and a blog with a gate of 1 would then answer
// one visitor while serving "503 busy" to everyone else for half a minute. That is strictly
// worse than the queueing it was meant to fix. The gate below therefore covers the retrieval
// call and nothing else — see askRetrieveGuarded, whose `defer` closes over exactly that call.
//
// 博客问答的检索并发闸。
//
// 一次提问有两个资源性质相反的阶段。检索——嵌入问题、探测 Bleve、算向量、调交叉编码器——
// 耗时 2–3 秒，且是纯烧钱：每次提问都记一次嵌入往返和一次重排往返，钱在没人读答案时也照样花掉。
// 生成耗时 10–30 秒，而本地几乎不花什么：它占一条上游连接，不占 CPU。
//
// 因此两个阶段需要不同的限制。检索拿一个小的全局闸；生成拿一把按访客的锁（在路由层执行，
// 因为客户端身份在那里），且不要全局上限。
//
// 闸必须「窄」的理由值得直说，因为显而易见的实现恰好是错的那个：用一个全局信号量包住整个请求，
// 它会在长达 10–30 秒的生成期间一直被持有，于是一个闸容量为 1 的博客会给一位访客作答，
// 同时向其他所有人回半分钟的「503 繁忙」。那比它想修的排队问题严格来说更糟。
// 因此下面的闸只覆盖检索调用，别无其他——见 askRetrieveGuarded，它的 defer 作用域正好是那一次调用。

// askRetrievalConcurrency is how many retrieval pipelines may run at once.
//
// Two, on a 2-core / 2 GB box: one question's retrieval is a handful of HTTP round trips plus
// a Bleve probe and a cosine scan, so two is enough to keep a second visitor from waiting
// behind the first, while a third still gets a fast "try again" instead of a slow queue. Being
// small is the point — the cost of a wrong answer here is a paid upstream call, not CPU.
//
// askRetrievalConcurrency 是同时允许运行的检索管线数。
//
// 在 2 核 2 G 的机器上取 2：一次检索是若干次 HTTP 往返加上一次 Bleve 探测和一次余弦扫描，
// 2 个足以让第二位访客不必等第一位，而第三位仍然得到快速的「稍后再试」而不是缓慢排队。
// 小就是目的——这里判断错误的代价是一次付费的上游调用，而不是 CPU。
const askRetrievalConcurrency = 2

// ErrAskRetrievalBusy reports that every retrieval slot was taken.
//
// It is a distinct error rather than a generic one because the caller must render it as
// "busy, try again" instead of as a failure: nothing went wrong, the blog is simply at capacity
// for two or three seconds.
//
// ErrAskRetrievalBusy 表示所有检索名额都被占用。
//
// 它必须是独立的错误而不是笼统的一种，因为调用方要把它渲染成「繁忙，请稍后再试」而不是「出错了」：
// 没有任何东西坏掉，博客只是在这两三秒里满负荷。
var ErrAskRetrievalBusy = errors.New("blog ask: retrieval capacity exhausted")

// retrievalGate returns the semaphore, creating it on first use.
//
// Lazily rather than only in the constructor so a service value built directly in a test still
// has a working gate instead of a nil channel that would make every question block forever on
// send.
//
// retrievalGate 返回信号量，首次使用时创建。
//
// 采用惰性创建而不是只在构造函数里建，是为了让测试里直接构造的服务值也有一个可用的闸，
// 而不是一个 nil channel——向 nil channel 发送会让每个问题永久阻塞。
func (s *shareService) retrievalGate() chan struct{} {
	s.askMu.Lock()
	defer s.askMu.Unlock()
	if s.askRetrievalSem == nil {
		s.askRetrievalSem = make(chan struct{}, askRetrievalConcurrency)
	}
	return s.askRetrievalSem
}

// askRetrieveGuarded runs fn only if a retrieval slot is free, and returns the slot as soon as
// fn returns.
//
// The whole point is the scope of the `defer`: it releases when the *retrieval* finishes, not
// when the answer finishes, so a question that is busy streaming its answer to one reader does
// not deny retrieval to another. Acquiring is non-blocking (the `default` arm), because a
// visitor would rather be told "busy" in 5 ms than wait behind a slow upstream embed call.
//
// askRetrieveGuarded 仅在有检索名额时运行 fn，并在 fn 返回时立刻归还名额。
//
// 全部要点在于 defer 的作用域：它在「检索」结束时释放，而不是在「回答」结束时，
// 因此一个正在给某位读者流式输出答案的问题，不会让另一位读者的检索被拒。
// 获取是非阻塞的（default 分支），因为访客更愿意在 5 毫秒内被告知「繁忙」，
// 而不是排在一次缓慢的上游嵌入调用后面。
func (s *shareService) askRetrieveGuarded(fn func() ([]*askHit, string)) ([]*askHit, string, error) {
	gate := s.retrievalGate()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	default:
		return nil, "", ErrAskRetrievalBusy
	}
	hits, channels := fn()
	return hits, channels, nil
}
