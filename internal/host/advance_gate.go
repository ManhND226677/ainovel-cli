package host

import (
	"fmt"
	"slices"

	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/flow"
	"github.com/voocel/ainovel-cli/internal/store"
)

// ChapterAdvanceGate 是 Host 唯一的创作前进政策组件：
//   - AdvanceHold：执行本次干预签署的一次性暂停；
//   - review permit：阻止未获许可的正向新章。
//
// 它不参与 Route，不解释 Task/Reason，也不做文学判断。
type ChapterAdvanceGate struct {
	store  *store.Store
	pause  func(reason string)
	report func(level, summary string)
}

func NewChapterAdvanceGate(s *store.Store, pause func(reason string), report func(level, summary string)) *ChapterAdvanceGate {
	return &ChapterAdvanceGate{store: s, pause: pause, report: report}
}

// HandleBoundary 消费命中的 hold，并对账章节许可。返回 true 表示 Engine 必须停止。
// auto 且无 hold 时只读一次 RunMeta，不触碰 Progress/PendingCommit/checkpoint。
func (g *ChapterAdvanceGate) HandleBoundary() bool {
	if g == nil || g.store == nil {
		return false
	}
	meta, err := g.store.RunMeta.Load()
	if err != nil {
		return g.fail(fmt.Errorf("read RunMeta: %w", err))
	}
	if meta == nil {
		return g.fail(fmt.Errorf("RunMeta missing"))
	}
	if !meta.AdvanceMode.Valid() {
		return g.fail(&domain.UnsupportedAdvanceModeError{Mode: meta.AdvanceMode})
	}
	if meta.AdvanceMode == domain.ChapterAdvanceAuto && meta.AdvancePermitChapter != 0 {
		return g.fail(fmt.Errorf("auto mode has leftover permit for chapter %d", meta.AdvancePermitChapter))
	}

	if meta.AdvanceHold != nil {
		if g.handleHold(*meta.AdvanceHold) {
			return true
		}
		// handleHold 可能消费完本 hold；继续对账 permit。
	}
	if meta.AdvanceMode == domain.ChapterAdvanceAuto {
		return false
	}
	return g.reconcilePermit(meta.AdvancePermitChapter)
}

func (g *ChapterAdvanceGate) handleHold(hold domain.AdvanceHold) bool {
	progress, err := g.store.Progress.Load()
	if err != nil {
		return g.fail(fmt.Errorf("read Progress to resolve one-time pause: %w", err))
	}
	resolution, err := flow.ResolveAdvanceHold(&hold, progress)
	if err != nil {
		return g.fail(err)
	}
	switch resolution {
	case flow.AdvanceHoldKeep:
		return false
	case flow.AdvanceHoldConsume:
		if err := g.store.RunMeta.ClearAdvanceHold(hold); err != nil {
			return g.fail(fmt.Errorf("consume one-time pause: %w", err))
		}
		g.reportEvent("info", withAdvanceReason("全书已完结，一次性暂停意图已解除", hold.Reason))
		return false
	case flow.AdvanceHoldConsumeAndStop:
		if err := g.store.RunMeta.ClearAdvanceHold(hold); err != nil {
			return g.fail(fmt.Errorf("consume one-time pause: %w", err))
		}
		msg := "已按用户要求在当前工作边界暂停"
		if hold.After == domain.AdvanceHoldAfterRewritesDrained {
			msg = "返工队列已排空，已暂停等待验收"
		}
		g.pauseNow(withAdvanceReason(msg, hold.Reason))
		return true
	default:
		return g.fail(fmt.Errorf("unknown one-time pause resolution %d", resolution))
	}
}

func (g *ChapterAdvanceGate) reconcilePermit(permit int) bool {
	if permit == 0 {
		return false
	}
	if permit < 0 {
		return g.fail(fmt.Errorf("chapter permit cannot be negative: %d", permit))
	}
	progress, err := g.store.Progress.Load()
	if err != nil {
		return g.fail(fmt.Errorf("read Progress to reconcile chapter permit: %w", err))
	}
	if progress == nil {
		return g.fail(fmt.Errorf("missing Progress, cannot reconcile permit for chapter %d", permit))
	}
	pending, err := g.store.Signals.LoadPendingCommit()
	if err != nil {
		return g.fail(fmt.Errorf("read PendingCommit to reconcile chapter permit: %w", err))
	}
	completed := slices.Contains(progress.CompletedChapters, permit)
	if completed {
		if pending != nil {
			if pending.Chapter != permit {
				return g.fail(fmt.Errorf("permit for chapter %d conflicts with PendingCommit for chapter %d", permit, pending.Chapter))
			}
			return false
		}
		if g.store.Checkpoints.LatestByStep(domain.ChapterScope(permit), "commit") == nil {
			return g.fail(fmt.Errorf("chapter %d marked completed but missing commit checkpoint", permit))
		}
		if err := g.store.RunMeta.ClearAdvancePermit(permit); err != nil {
			return g.fail(fmt.Errorf("consume permit for chapter %d: %w", permit, err))
		}
		return false
	}
	if permit != progress.NextChapter() {
		return g.fail(fmt.Errorf("permit for chapter %d inconsistent with current next chapter %d", permit, progress.NextChapter()))
	}
	return false
}

// Allow 在 Worker 派发前执行最终许可检查。
func (g *ChapterAdvanceGate) Allow(inst *flow.Instruction) (bool, error) {
	if g == nil || g.store == nil {
		return true, nil
	}
	meta, err := g.store.RunMeta.Load()
	if err != nil {
		return false, fmt.Errorf("read RunMeta: %w", err)
	}
	if meta == nil {
		return false, fmt.Errorf("RunMeta missing")
	}
	if !meta.AdvanceMode.Valid() {
		return false, &domain.UnsupportedAdvanceModeError{Mode: meta.AdvanceMode}
	}
	if meta.AdvanceMode == domain.ChapterAdvanceAuto {
		if meta.AdvancePermitChapter != 0 {
			return false, fmt.Errorf("auto mode has leftover permit for chapter %d", meta.AdvancePermitChapter)
		}
		return true, nil
	}
	progress, err := g.store.Progress.Load()
	if err != nil {
		return false, fmt.Errorf("read Progress: %w", err)
	}
	pending, err := g.store.Signals.LoadPendingCommit()
	if err != nil {
		return false, fmt.Errorf("read PendingCommit: %w", err)
	}
	if !flow.StartsForwardChapter(inst, progress, pending) {
		return true, nil
	}
	target := inst.Chapter
	if target == 0 {
		target = progress.NextChapter()
	}
	if meta.AdvancePermitChapter == target {
		return true, nil
	}
	if meta.AdvancePermitChapter != 0 {
		return false, fmt.Errorf("dispatch for chapter %d inconsistent with permit for chapter %d", target, meta.AdvancePermitChapter)
	}
	latest := progress.LatestCompleted()
	message := fmt.Sprintf("已完成至第 %d 章，逐章验收等待放行第 %d 章；使用 /next 生成，或输入修改意见", latest, target)
	if latest == 0 {
		message = fmt.Sprintf("规划已就绪，逐章验收等待放行第 %d 章；使用 /next 生成，或输入修改意见", target)
	}
	g.pauseNow(message)
	return false, nil
}

func (g *ChapterAdvanceGate) fail(err error) bool {
	g.pauseNow("章节推进控制错误，已暂停：" + err.Error())
	return true
}

func (g *ChapterAdvanceGate) pauseNow(reason string) {
	if g.pause != nil {
		g.pause(reason)
		return
	}
	g.reportEvent("error", reason)
}

func (g *ChapterAdvanceGate) reportEvent(level, summary string) {
	if g.report != nil {
		g.report(level, summary)
	}
}

func withAdvanceReason(msg, reason string) string {
	if reason == "" {
		return msg
	}
	return msg + "（诉求：" + reason + "）"
}
