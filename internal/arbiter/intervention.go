package arbiter

import (
	"context"
	"fmt"
	"strings"

	"github.com/voocel/agentcore"
	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/llmcontract"
	storepkg "github.com/voocel/ainovel-cli/internal/store"
)

// InterventionFacts 干预分诊的事实包(Collect 时刻快照)。
// Engine 在边界执行 Dispatch 前用 Phase/QueueHead 做对账(咨询与执行之间隔着
// worker 运行,事实可能已推进;不符 → 丢弃并以新事实重询)。
type InterventionFacts struct {
	Phase             string           `json:"phase,omitempty"`
	Flow              string           `json:"flow,omitempty"`
	NovelName         string           `json:"novel_name,omitempty"`
	CompletedChapters int              `json:"completed_chapters"`
	OutlinedChapters  int              `json:"outlined_chapters,omitempty"`
	DynamicPlanning   bool             `json:"dynamic_planning"`
	NextChapter       int              `json:"next_chapter,omitempty"`
	PendingRewrites   []int            `json:"pending_rewrites,omitempty"`
	ReopenCount       int              `json:"reopen_count,omitempty"` // 用户显式 /reopen 重开完结书的累计次数
	FoundationMissing []string         `json:"foundation_missing,omitempty"`
	PlanningTier      string           `json:"planning_tier,omitempty"`
	AdvanceMode       string           `json:"advance_mode,omitempty"`
	HasAdvanceHold    bool             `json:"has_advance_hold"`
	AdvanceHoldAfter  string           `json:"advance_hold_after,omitempty"`
	AdvanceHoldReason string           `json:"advance_hold_reason,omitempty"`
	Running           bool             `json:"running"`                  // 干预到达时是否有 run 在进行
	CheckpointSeq     int64            `json:"checkpoint_seq,omitempty"` // Collect 时刻最新 checkpoint;Engine 对账用
	RecentDecisions   []RecentDecision `json:"recent_decisions,omitempty"`
}

// RecentDecision 是干预记忆:最近几次裁定的摘要,覆盖"上次改的怎么样了"类跨干预引用。
type RecentDecision struct {
	At     string `json:"at"`
	Input  string `json:"input"`
	Reason string `json:"reason,omitempty"`
}

// QueueHead 返回重写队列头(无则 0),Engine 对账用。
func (f InterventionFacts) QueueHead() int {
	if len(f.PendingRewrites) > 0 {
		return f.PendingRewrites[0]
	}
	return 0
}

// CollectInterventionFacts 从 store 读齐分诊事实。任何控制事实读取失败都显式
// 返回错误，禁止 Arbiter 在零值拼成的不完整快照上做语义决策。
func CollectInterventionFacts(st *storepkg.Store) (InterventionFacts, error) {
	var f InterventionFacts
	if st == nil {
		return f, fmt.Errorf("cửa hàng (store) không được để trống")
	}
	missing, err := st.FoundationMissing()
	if err != nil {
		return f, fmt.Errorf("không thể tải trạng thái cài đặt cơ bản: %w", err)
	}
	f.FoundationMissing = missing
	p, err := st.Progress.Load()
	if err != nil {
		return f, fmt.Errorf("không thể tải tiến trình: %w", err)
	}
	if p != nil {
		f.Phase = string(p.Phase)
		f.Flow = string(p.Flow)
		f.NovelName = p.NovelName
		f.CompletedChapters = len(p.CompletedChapters)
		f.DynamicPlanning = p.Layered
		if p.Layered {
			outline, outlineErr := st.Outline.LoadOutline()
			if outlineErr != nil {
				return f, fmt.Errorf("không thể tải đề cương chi tiết hiện tại: %w", outlineErr)
			}
			f.OutlinedChapters = len(outline)
		} else {
			f.OutlinedChapters = p.TotalChapters
		}
		f.NextChapter = p.NextChapter()
		f.PendingRewrites = append([]int(nil), p.PendingRewrites...)
		f.ReopenCount = p.ReopenCount
	}
	meta, err := st.RunMeta.Load()
	if err != nil {
		return f, fmt.Errorf("không thể tải thông tin chạy: %w", err)
	}
	if meta != nil {
		f.PlanningTier = string(meta.PlanningTier)
		f.AdvanceMode = string(meta.AdvanceMode)
		if meta.AdvanceHold != nil {
			f.HasAdvanceHold = true
			f.AdvanceHoldAfter = string(meta.AdvanceHold.After)
			f.AdvanceHoldReason = meta.AdvanceHold.Reason
		}
	}
	if cp := st.Checkpoints.LatestGlobal(); cp != nil {
		f.CheckpointSeq = cp.Seq
	}
	recent, err := st.Decisions.Recent(5)
	if err != nil {
		return f, fmt.Errorf("không thể tải các quyết định gần đây: %w", err)
	}
	for _, r := range recent {
		if r.Kind != "intervention" {
			continue
		}
		f.RecentDecisions = append(f.RecentDecisions, RecentDecision{
			At: r.At, Input: truncateRunes(r.Input, 80), Reason: r.Reason,
		})
	}
	return f, nil
}

// AdvanceHoldOp 一次性暂停动作：在 Worker 边界或返工排空后暂停，也可取消。
type AdvanceHoldOp struct {
	Cancel bool                    `json:"cancel,omitempty"`
	After  domain.AdvanceHoldAfter `json:"after,omitempty"`
	Reason string                  `json:"reason,omitempty"`
}

// ReopenOp 完本返工:把全书重开进返工态并把目标章入队(仅 phase=complete 合法)。
type ReopenOp struct {
	Chapters []int  `json:"chapters"`
	Reason   string `json:"reason,omitempty"`
}

// InterventionDecision 干预裁定。动作组合自由,执行顺序由 Engine 固定:
// answer → rules → hold → reopen → dispatch;至多一个 dispatch(类型事实)。
type InterventionDecision struct {
	Answer   string         `json:"answer,omitempty"`
	Rules    string         `json:"rules,omitempty"`
	Hold     *AdvanceHoldOp `json:"hold,omitempty"`
	Reopen   *ReopenOp      `json:"reopen,omitempty"`
	Dispatch *DispatchOp    `json:"dispatch,omitempty"`
	Reason   string         `json:"reason"`
}

var interventionContract = llmcontract.Contract{
	Name:        "arbiter_intervention",
	Description: "Phán quyết can thiệp của người dùng: phản hồi, quy tắc, tạm dừng, mở lại và phái cử",
	Schema: schema.Object(
		schema.Property("answer", llmcontract.Nullable(schema.String("Văn bản phản hồi cho người dùng; nếu không có thì null"))).Required(),
		schema.Property("rules", llmcontract.Nullable(schema.String("Nguyên bản các quy tắc viết văn bản dài sẽ được ghi vào đĩa; nếu không có thì null"))).Required(),
		schema.Property("hold", llmcontract.Nullable(schema.Object(
			schema.Property("cancel", schema.Bool("Có hủy bỏ việc tạm dừng một lần hiện tại không")).Required(),
			schema.Property("after", llmcontract.Nullable(schema.Enum("Điểm kích hoạt tạm dừng; null khi hủy bỏ", string(domain.AdvanceHoldAtBoundary), string(domain.AdvanceHoldAfterRewritesDrained)))).Required(),
			schema.Property("reason", llmcontract.Nullable(schema.String("Tóm tắt yêu cầu của người dùng; có thể null khi hủy bỏ"))).Required(),
		))).Required(),
		schema.Property("reopen", llmcontract.Nullable(schema.Object(
			schema.Property("chapters", schema.Array("Số chương cần mở lại", schema.Int("Số chương"))).Required(),
			schema.Property("reason", llmcontract.Nullable(schema.String("Lý do mở lại"))).Required(),
		))).Required(),
		schema.Property("dispatch", dispatchSchema("Mục tiêu phái cử; nếu không cần thì null")).Required(),
		schema.Property("reason", schema.String("Lý do phán quyết ngắn gọn")).Required(),
	),
}

// ValidateAgainst 按事实做机械校验(场景内合法性;类型已排除跨场景动作)。
func (d *InterventionDecision) ValidateAgainst(f InterventionFacts) error {
	if strings.TrimSpace(d.Reason) == "" {
		return fmt.Errorf("reason không được để trống")
	}
	if d.Answer == "" && d.Rules == "" && d.Hold == nil && d.Reopen == nil && d.Dispatch == nil {
		return fmt.Errorf("quyết định trống: phải có ít nhất một hành động hoặc answer")
	}
	if err := d.Dispatch.validate(); err != nil {
		return err
	}
	if err := validateDispatchAgainst(d.Dispatch, f.Phase); err != nil {
		return err
	}
	complete := f.Phase == string(domain.PhaseComplete)
	if d.Reopen != nil {
		if !complete {
			return fmt.Errorf("reopen chỉ được sử dụng trong giai đoạn hoàn thành (phase hiện tại=%s)", f.Phase)
		}
		if len(d.Reopen.Chapters) == 0 {
			return fmt.Errorf("reopen.chapters không được để trống")
		}
		for _, ch := range d.Reopen.Chapters {
			if ch < 1 || ch > f.CompletedChapters {
				return fmt.Errorf("chương cần mở lại %d vượt quá giới hạn (đã hoàn thành %d chương)", ch, f.CompletedChapters)
			}
		}
	}
	if complete && d.Dispatch != nil {
		return fmt.Errorf("không thể trực tiếp phái cử trong giai đoạn hoàn thành; dùng reopen để sửa lại (sau khi đưa vào hàng đợi, Router sẽ tự động phân phối)")
	}
	if d.Hold != nil && !d.Hold.Cancel {
		if f.Phase != string(domain.PhaseWriting) {
			return fmt.Errorf("tạm dừng một lần chỉ có thể được thiết lập trong giai đoạn viết (phase hiện tại=%s)", f.Phase)
		}
		if !d.Hold.After.Valid() {
			return fmt.Errorf("hold.after phải là boundary hoặc rewrites_drained")
		}
		if strings.TrimSpace(d.Hold.Reason) == "" {
			return fmt.Errorf("thiết lập tạm dừng một lần phải có reason (tóm tắt yêu cầu của người dùng)")
		}
	}
	return nil
}

// validateDispatchAgainst 把提示词中的阶段纪律落实为机械防线。Architect 可在规划期
// 与写作期维护结构；Writer/Editor 只能消费已经完整且进入 writing 的作品事实。
func validateDispatchAgainst(dispatch *DispatchOp, phase string) error {
	if dispatch == nil {
		return nil
	}
	if phase == "" {
		return fmt.Errorf("thiếu phase, cấm phái cử")
	}
	if phase == string(domain.PhaseComplete) {
		return fmt.Errorf("không được phép phái cử trực tiếp trong giai đoạn hoàn thành")
	}
	switch dispatch.Agent {
	case "writer", "editor":
		if phase != string(domain.PhaseWriting) {
			return fmt.Errorf("%s chỉ có thể được phân công ở giai đoạn writing (phase hiện tại=%s)", dispatch.Agent, phase)
		}
	}
	return nil
}

// DecideIntervention 干预分诊。失败语义:返回 error → 调用方显式回显
// 真实失败原因,且不产生任何写入(宁可不动,不可误动)。
func DecideIntervention(ctx context.Context, model agentcore.ChatModel, systemPrompt string, facts InterventionFacts, text string) (InterventionDecision, error) {
	payload, err := marshalPayload(struct {
		Intervention string            `json:"intervention"`
		Facts        InterventionFacts `json:"facts"`
	}{Intervention: text, Facts: facts})
	if err != nil {
		return InterventionDecision{}, err
	}
	return decide(ctx, model, interventionContract, systemPrompt, payload, func(d *InterventionDecision) error {
		return d.ValidateAgainst(facts)
	})
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
