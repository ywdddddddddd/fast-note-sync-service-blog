package service

import (
	"errors"
	"sync"
	"testing"
)

// TestAskRetrieveGuardedLimitsConcurrency pins the two properties the retrieval gate exists for.
//
// First: at most askRetrievalConcurrency retrievals run at once, and the one that cannot get a
// slot fails immediately with ErrAskRetrievalBusy instead of queueing. Second — and this is the
// property a wrong implementation loses — the slot comes back when the *retrieval* returns, not
// when the caller finishes. That is what keeps a question that is busy streaming its answer to
// one reader from denying retrieval to another for half a minute.
//
// TestAskRetrieveGuardedLimitsConcurrency 锁住检索闸存在的两条性质。
//
// 其一：同一时刻最多 askRetrievalConcurrency 次检索在跑，拿不到名额的那一个立刻以
// ErrAskRetrievalBusy 失败，而不是排队。其二——这正是错误实现会丢掉的那条——名额在「检索」
// 返回时就归还，而不是在调用方结束时。正是它使一个正在给某位读者流式输出答案的问题，
// 不会在接下来的半分钟里让另一位读者的检索被拒。
func TestAskRetrieveGuardedLimitsConcurrency(t *testing.T) {
	s := &shareService{}

	hold := make(chan struct{})
	acquired := make(chan struct{}, askRetrievalConcurrency)
	var wg sync.WaitGroup

	for i := 0; i < askRetrievalConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.askRetrieveGuarded(func() ([]*askHit, string) {
				acquired <- struct{}{}
				<-hold
				return nil, "held"
			}); err != nil {
				t.Errorf("占位检索返回错误: %v", err)
			}
		}()
	}

	// Both slots are provably taken once both bodies are running.
	// 两个名额在两侧函数体都跑起来后可证明已被占满。
	for i := 0; i < askRetrievalConcurrency; i++ {
		<-acquired
	}

	if _, _, err := s.askRetrieveGuarded(func() ([]*askHit, string) {
		t.Error("名额已满时仍然执行了检索体")
		return nil, ""
	}); !errors.Is(err, ErrAskRetrievalBusy) {
		t.Fatalf("名额占满时应返回 ErrAskRetrievalBusy，实际 %v", err)
	}

	close(hold)
	wg.Wait()

	// Slots must be free again after the retrieval bodies returned.
	// 检索函数体返回后，名额必须重新可用。
	ran := false
	hits, channels, err := s.askRetrieveGuarded(func() ([]*askHit, string) {
		ran = true
		return nil, "free"
	})
	if err != nil {
		t.Fatalf("名额应已释放，实际返回错误: %v", err)
	}
	if !ran || channels != "free" || hits != nil {
		t.Fatalf("释放后的检索未按预期执行: ran=%v channels=%q", ran, channels)
	}
}

// TestAskRetrieveGuardedReleasesOnPanic guards the defer, not the happy path.
//
// A panic inside retrieval must not permanently consume a slot: with two slots and a vault that
// panics deterministically, leaking one slot per occurrence would silence the feature after two
// questions — and it would look like a rate-limit bug rather than a crash.
//
// TestAskRetrieveGuardedReleasesOnPanic 守的是 defer，而不是正常路径。
//
// 检索内部 panic 不能永久吃掉一个名额：只有两个名额，若一份必然 panic 的语料每崩一次漏掉
// 一个名额，问两个问题后功能就会静默失效——而且它看起来像限流缺陷，而不是崩溃。
func TestAskRetrieveGuardedReleasesOnPanic(t *testing.T) {
	s := &shareService{}

	func() {
		defer func() { _ = recover() }()
		_, _, _ = s.askRetrieveGuarded(func() ([]*askHit, string) {
			panic("retrieval exploded")
		})
	}()

	done := make(chan error, 1)
	go func() {
		_, _, err := s.askRetrieveGuarded(func() ([]*askHit, string) { return nil, "ok" })
		done <- err
	}()
	if err := <-done; err != nil {
		t.Fatalf("panic 之后名额未归还: %v", err)
	}
}
