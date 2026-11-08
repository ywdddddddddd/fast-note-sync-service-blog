package routers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/haierkeys/fast-note-sync-service/internal/service"
)

// TestRunAskStreamDeadlineSendsClosingEvent pins the deadline path.
//
// The HTTP status is already committed once the first stage event leaves, so the only honest
// ending for an over-budget answer is one last `error` frame. The injected budget is
// milliseconds: the property under test is that the context deadline actually interrupts the
// ask function and that the closing frame names the interruption, not that sixty seconds is
// sixty seconds.
//
// TestRunAskStreamDeadlineSendsClosingEvent 钉住截止路径。
//
// 第一帧 stage 离开后 HTTP 状态就已提交，因此超预算的回答唯一诚实的结局是最后一帧 `error`。
// 注入的预算是毫秒级：被测性质是上下文截止真的打断了 ask 函数、收尾帧说明了中断，
// 而不是「六十秒就是六十秒」。
func TestRunAskStreamDeadlineSendsClosingEvent(t *testing.T) {
	var events []service.AskEvent
	start := time.Now()
	runAskStream(context.Background(), 20*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, func(ev service.AskEvent) error {
		events = append(events, ev)
		return nil
	})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("20ms 的注入预算却等了 %v", elapsed)
	}
	if len(events) != 1 {
		t.Fatalf("应恰好发一条收尾事件，实际 %d 条：%+v", len(events), events)
	}
	if events[0].Type != "error" {
		t.Fatalf("收尾事件类型应为 error，实际 %q", events[0].Type)
	}
	if !strings.Contains(events[0].Message, "自动中断") {
		t.Fatalf("收尾文案应说明已自动中断，实际 %q", events[0].Message)
	}
}

// TestRunAskStreamDeadlineAfterPartialAnswer reproduces the case that matters in production:
// the upstream streamed part of an answer and then went silent past the budget. The closing
// frame must still arrive; otherwise the browser sits on a half-written answer forever.
//
// TestRunAskStreamDeadlineAfterPartialAnswer 复现线上真正要紧的情形：上游输出了一半答案后
// 沉默超过预算。收尾帧仍必须到达，否则浏览器会永远停在写了一半的答案上。
func TestRunAskStreamDeadlineAfterPartialAnswer(t *testing.T) {
	var emitted int
	var closing string
	runAskStream(context.Background(), 20*time.Millisecond, func(ctx context.Context) error {
		emitted++
		<-ctx.Done()
		return ctx.Err()
	}, func(ev service.AskEvent) error {
		if ev.Type == "delta" {
			emitted++
		}
		if ev.Type == "error" {
			closing = ev.Message
		}
		return nil
	})
	if emitted == 0 {
		t.Fatal("ask 函数没有运行")
	}
	if !strings.Contains(closing, "自动中断") {
		t.Fatalf("部分答案后仍应收到中断收尾，实际 %q", closing)
	}
}

// TestRunAskStreamDeadlineEvenWhenAskReturnsNil covers the service's actual shape: when the
// context expires mid-route it emits its own error frame and returns nil. If the helper only
// looked at the returned error, the reader would get the service's transport sentence instead
// of the timeout sentence, or — worse — no final frame at all.
//
// TestRunAskStreamDeadlineEvenWhenAskReturnsNil 覆盖服务真实的返回形态：上下文在线路中途过期时，
// 服务自己发一帧 error 然后返回 nil。若辅助函数只看返回值，读者会看到服务的传输错误句，
// 或者更糟——根本没有收尾帧。
func TestRunAskStreamDeadlineEvenWhenAskReturnsNil(t *testing.T) {
	var closing string
	runAskStream(context.Background(), 20*time.Millisecond, func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}, func(ev service.AskEvent) error {
		if ev.Type == "error" {
			closing = ev.Message
		}
		return nil
	})
	if !strings.Contains(closing, "自动中断") {
		t.Fatalf("服务返回 nil 时也必须补一条超时收尾，实际 %q", closing)
	}
}

// TestRunAskStreamParentCancelSendsNothing covers the reader who closed the tab. Writing an
// error frame into a dead connection is pointless, and "the client went away" is not an error
// the remaining reader needs to see — there is no remaining reader.
//
// TestRunAskStreamParentCancelSendsNothing 覆盖关掉标签页的读者。往已经断开的连接里写错误帧
// 没有意义，而「客户端走了」也不是仍需阅读的人该看到的错误——已经没有人在读了。
func TestRunAskStreamParentCancelSendsNothing(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	called := 0
	runAskStream(parent, time.Second, func(ctx context.Context) error {
		called++
		return ctx.Err()
	}, func(service.AskEvent) error {
		t.Fatal("父上下文取消时不应再写任何事件")
		return nil
	})
	if called != 1 {
		t.Fatalf("ask 函数应被调用一次，实际 %d 次", called)
	}
}

// TestRunAskStreamOrdinaryErrorIsForwarded verifies the non-deadline path still reports the
// cause, so a broken line is visible instead of looking like a silent screen.
//
// TestRunAskStreamOrdinaryErrorIsForwarded 验证非截止路径仍会报出原因，
// 使断掉的线路可见，而不是表现为一块沉默的屏幕。
func TestRunAskStreamOrdinaryErrorIsForwarded(t *testing.T) {
	var events []service.AskEvent
	runAskStream(context.Background(), time.Second, func(context.Context) error {
		return errors.New("upstream exploded")
	}, func(ev service.AskEvent) error {
		events = append(events, ev)
		return nil
	})
	if len(events) != 1 || events[0].Type != "error" || events[0].Message != "upstream exploded" {
		t.Fatalf("普通错误应原样转发一帧，实际 %+v", events)
	}
}

// TestRunAskStreamSuccessWritesNothing keeps the helper out of the happy path: a normal answer
// already ends with its own `done` frame, and adding a second one would look like two answers.
//
// TestRunAskStreamSuccessWritesNothing 让辅助函数不介入正常路径：正常回答已有自己的 `done` 帧，
// 再补一帧看起来会像有两个回答。
func TestRunAskStreamSuccessWritesNothing(t *testing.T) {
	runAskStream(context.Background(), time.Second, func(context.Context) error {
		return nil
	}, func(service.AskEvent) error {
		t.Fatal("成功路径不应写收尾事件")
		return nil
	})
}
