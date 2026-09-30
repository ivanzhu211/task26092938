// Package bottleneck 提供审批瓶颈分析能力：
// 由一份冻结的审批台账建立只读统计快照，并支持按最终完成时间窗口查询。
package bottleneck

// Status 表示提交的最终/当前状态。
type Status int

const (
	StatusDraft    Status = iota // 草稿
	StatusPending                // 待审批
	StatusReturned               // 退回修改
	StatusApproved               // 已批准
	StatusRejected               // 已拒绝
)

// Stay 是一次阶段停留记录：进入与离开时间，UTC 整数秒。
type Stay struct {
	Stage   string
	EnterAt int64
	LeaveAt int64
}

// Submission 是台账中的一份提交。
type Submission struct {
	ID          string
	Status      Status
	CompletedAt int64 // 最终完成时间，UTC 整数秒
	Stays       []Stay
}

// StageStats 是单个阶段在查询窗口内的统计结果。
type StageStats struct {
	Stage   string
	Samples int     // 样本数（到达过该阶段的提交数）
	Average float64 // 平均停留秒数
	Median  float64 // 精确中位数
}

// WindowResult 是一次窗口查询的结果。
type WindowResult struct {
	Stages      []StageStats // 仅包含有样本的阶段
	Bottlenecks []string     // 中位数最大的阶段，并列时全部返回
}
