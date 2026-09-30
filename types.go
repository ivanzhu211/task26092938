// Package bottleneck 提供审批瓶颈分析能力：
// 由上层一次性导入冻结台账，建立只读统计快照，
// 随后按提交最终完成时间窗口反复查询各阶段停留统计与瓶颈阶段。
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

// MaxStages 为单份台账允许的最大阶段数。
const MaxStages = 10

// StageStay 表示一次阶段停留记录，时间为 UTC 整数秒。
type StageStay struct {
	Stage int   // 阶段下标，取值 [0, stageCount)
	Enter int64 // 进入时间（含）
	Leave int64 // 离开时间（含），Leave >= Enter
}

// Submission 表示台账中的一份提交及其完整阶段历史。
type Submission struct {
	ID          string      // 唯一标识
	Status      Status      // 当前状态
	CompletedAt int64       // 最终完成时间（UTC 秒）
	Stays       []StageStay // 完整阶段进入/离开记录，可无序
}

// StageStats 表示单个阶段在查询窗口内的统计结果。
type StageStats struct {
	Stage   int     // 阶段下标
	Samples int     // 样本数（实际到达该阶段的选中提交数）
	Average float64 // 平均停留秒数
	Median  float64 // 精确中位数（偶数样本取中央两值算术平均）
}

// WindowResult 表示一次窗口查询的结果。
type WindowResult struct {
	Stages     []StageStats // 有样本的阶段，按下标升序；无样本阶段不出现
	Bottleneck []int        // 中位数最大的阶段下标，并列时全部返回，升序
}
