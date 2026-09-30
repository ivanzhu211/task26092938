package bottleneck

import (
	"math/rand"
	"sort"
	"testing"
)

// 验收用例：一份提交在阶段A两次停留2秒和8秒，另两份分别停留4秒、6秒。
func TestAcceptanceReworkAggregation(t *testing.T) {
	snap, err := BuildSnapshot([]Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 100, Stays: []StageStay{
			{Stage: 0, Enter: 10, Leave: 12},
			{Stage: 0, Enter: 50, Leave: 58},
		}},
		{ID: "s2", Status: StatusApproved, CompletedAt: 101, Stays: []StageStay{
			{Stage: 0, Enter: 20, Leave: 24},
		}},
		{ID: "s3", Status: StatusRejected, CompletedAt: 102, Stays: []StageStay{
			{Stage: 0, Enter: 30, Leave: 36},
		}},
	}, 1)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	res := snap.Query(100, 103)
	if len(res.Stages) != 1 {
		t.Fatalf("want 1 stage, got %d", len(res.Stages))
	}
	st := res.Stages[0]
	if st.Samples != 3 {
		t.Errorf("samples: want 3, got %d", st.Samples)
	}
	if want := 20.0 / 3.0; st.Average != want {
		t.Errorf("average: want %v, got %v", want, st.Average)
	}
	if st.Median != 6 {
		t.Errorf("median: want 6, got %v", st.Median)
	}
	if len(res.Bottleneck) != 1 || res.Bottleneck[0] != 0 {
		t.Errorf("bottleneck: want [0], got %v", res.Bottleneck)
	}
}

// 验收用例：完成时间落在相邻窗口公共边界时只进入后一个窗口，
// 且窗口开始前发生的阶段停留完整计入。
func TestWindowBoundary(t *testing.T) {
	snap, err := BuildSnapshot([]Submission{
		{ID: "edge", Status: StatusApproved, CompletedAt: 1000, Stays: []StageStay{
			{Stage: 0, Enter: 100, Leave: 107}, // 窗口开始前已完成的停留
		}},
	}, 1)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if got := snap.Query(0, 1000); len(got.Stages) != 0 || len(got.Bottleneck) != 0 {
		t.Errorf("first window: want empty, got %+v", got)
	}
	got := snap.Query(1000, 2000)
	if len(got.Stages) != 1 {
		t.Fatalf("second window: want 1 stage, got %+v", got)
	}
	if got.Stages[0].Samples != 1 || got.Stages[0].Average != 7 || got.Stages[0].Median != 7 {
		t.Errorf("second window: want samples=1 avg=7 median=7, got %+v", got.Stages[0])
	}
}

// 非终态提交整体排除，即使其中已有关闭的阶段记录。
func TestTerminalStatusFilter(t *testing.T) {
	snap, err := BuildSnapshot([]Submission{
		{ID: "d", Status: StatusDraft, CompletedAt: 10, Stays: []StageStay{{Stage: 0, Enter: 1, Leave: 9}}},
		{ID: "p", Status: StatusPending, CompletedAt: 11, Stays: []StageStay{{Stage: 0, Enter: 1, Leave: 9}}},
		{ID: "r", Status: StatusReturned, CompletedAt: 12, Stays: []StageStay{{Stage: 0, Enter: 1, Leave: 9}}},
	}, 2)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if got := snap.Query(0, 100); len(got.Stages) != 0 || len(got.Bottleneck) != 0 {
		t.Errorf("want empty result, got %+v", got)
	}
}

// 偶数样本中位数为中央两值的算术平均；并列瓶颈全部返回。
func TestEvenMedianAndTiedBottleneck(t *testing.T) {
	snap, err := BuildSnapshot([]Submission{
		{ID: "a", Status: StatusApproved, CompletedAt: 1, Stays: []StageStay{
			{Stage: 0, Enter: 0, Leave: 1}, {Stage: 1, Enter: 0, Leave: 3},
		}},
		{ID: "b", Status: StatusApproved, CompletedAt: 2, Stays: []StageStay{
			{Stage: 0, Enter: 0, Leave: 4}, {Stage: 1, Enter: 0, Leave: 1},
		}},
	}, 2)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	res := snap.Query(0, 10)
	if len(res.Stages) != 2 {
		t.Fatalf("want 2 stages, got %+v", res.Stages)
	}
	// 阶段0样本 {1,4} 中位数2.5；阶段1样本 {3,1} 中位数2，并列瓶颈不成立的场景
	if res.Stages[0].Median != 2.5 {
		t.Errorf("stage0 median: want 2.5, got %v", res.Stages[0].Median)
	}
	if res.Stages[1].Median != 2 {
		t.Errorf("stage1 median: want 2, got %v", res.Stages[1].Median)
	}
	if len(res.Bottleneck) != 1 || res.Bottleneck[0] != 0 {
		t.Errorf("bottleneck: want [0], got %v", res.Bottleneck)
	}

	// 构造两阶段中位数相同的并列场景
	snap2, err := BuildSnapshot([]Submission{
		{ID: "x", Status: StatusApproved, CompletedAt: 1, Stays: []StageStay{
			{Stage: 0, Enter: 0, Leave: 5}, {Stage: 1, Enter: 0, Leave: 5},
		}},
	}, 2)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	res2 := snap2.Query(0, 10)
	if len(res2.Bottleneck) != 2 || res2.Bottleneck[0] != 0 || res2.Bottleneck[1] != 1 {
		t.Errorf("tied bottleneck: want [0 1], got %v", res2.Bottleneck)
	}
}

// 未到达的阶段不补零；实际到达但停留为0秒的阶段仍贡献样本。
func TestZeroStayCountsButMissingStageDoesNot(t *testing.T) {
	snap, err := BuildSnapshot([]Submission{
		{ID: "z", Status: StatusApproved, CompletedAt: 1, Stays: []StageStay{
			{Stage: 1, Enter: 7, Leave: 7}, // 0秒停留，贡献样本
		}},
	}, 3)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	res := snap.Query(0, 10)
	if len(res.Stages) != 1 || res.Stages[0].Stage != 1 {
		t.Fatalf("want only stage 1, got %+v", res.Stages)
	}
	if res.Stages[0].Samples != 1 || res.Stages[0].Median != 0 || res.Stages[0].Average != 0 {
		t.Errorf("want zero-valued sample, got %+v", res.Stages[0])
	}
}

// 非法输入校验。
func TestInvalidInput(t *testing.T) {
	if _, err := BuildSnapshot(nil, 0); err == nil {
		t.Error("want error for stageCount 0")
	}
	if _, err := BuildSnapshot(nil, MaxStages+1); err == nil {
		t.Error("want error for stageCount > MaxStages")
	}
	if _, err := BuildSnapshot([]Submission{
		{ID: "bad", Status: StatusApproved, CompletedAt: 1, Stays: []StageStay{{Stage: 5, Enter: 0, Leave: 1}}},
	}, 2); err == nil {
		t.Error("want error for stage out of range")
	}
	if _, err := BuildSnapshot([]Submission{
		{ID: "neg", Status: StatusApproved, CompletedAt: 1, Stays: []StageStay{{Stage: 0, Enter: 5, Leave: 3}}},
	}, 2); err == nil {
		t.Error("want error for leave before enter")
	}
}

// 小规模随机历史与独立暴力计算的精确结果对照。
func TestAgainstBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 200; trial++ {
		stageCount := 1 + rng.Intn(MaxStages)
		var subs []Submission
		base := int64(1_000_000)
		for i := 0; i < 30; i++ {
			statuses := []Status{StatusApproved, StatusRejected, StatusDraft, StatusPending, StatusReturned}
			sub := Submission{
				ID:          string(rune('a'+i)) + string(rune('A'+rng.Intn(26))),
				Status:      statuses[rng.Intn(len(statuses))],
				CompletedAt: base + int64(rng.Intn(1000)),
			}
			for s := 0; s < stageCount; s++ {
				if rng.Intn(4) == 0 {
					continue // 条件审批跳过该阶段
				}
				rounds := 1 + rng.Intn(3) // 退回重提产生多轮
				for k := 0; k < rounds; k++ {
					enter := base - int64(rng.Intn(5000))
					sub.Stays = append(sub.Stays, StageStay{Stage: s, Enter: enter, Leave: enter + int64(rng.Intn(50))})
				}
			}
			subs = append(subs, sub)
		}
		snap, err := BuildSnapshot(subs, stageCount)
		if err != nil {
			t.Fatalf("trial %d: BuildSnapshot: %v", trial, err)
		}
		for q := 0; q < 20; q++ {
			start := base + int64(rng.Intn(1000))
			end := start + int64(rng.Intn(1000))
			got := snap.Query(start, end)
			want := bruteForce(subs, stageCount, start, end)
			assertWindowResultEqual(t, trial, start, end, want, got)
		}
	}
}

// bruteForce 按业务规则独立计算窗口结果。
func bruteForce(subs []Submission, stageCount int, start, end int64) WindowResult {
	res := WindowResult{}
	var maxMedian float64
	for stage := 0; stage < stageCount; stage++ {
		var samples []int64
		for _, sub := range subs {
			if sub.Status != StatusApproved && sub.Status != StatusRejected {
				continue
			}
			if sub.CompletedAt < start || sub.CompletedAt >= end {
				continue
			}
			var sum int64
			reached := false
			for _, st := range sub.Stays {
				if st.Stage == stage {
					sum += st.Leave - st.Enter
					reached = true
				}
			}
			if reached {
				samples = append(samples, sum)
			}
		}
		if len(samples) == 0 {
			continue
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		var total int64
		for _, v := range samples {
			total += v
		}
		n := len(samples)
		stats := StageStats{
			Stage:   stage,
			Samples: n,
			Average: float64(total) / float64(n),
			Median:  float64(samples[(n-1)/2])/2 + float64(samples[n/2])/2,
		}
		if len(res.Stages) == 0 || stats.Median > maxMedian {
			maxMedian = stats.Median
			res.Bottleneck = append(res.Bottleneck[:0], stage)
		} else if stats.Median == maxMedian {
			res.Bottleneck = append(res.Bottleneck, stage)
		}
		res.Stages = append(res.Stages, stats)
	}
	return res
}

func assertWindowResultEqual(t *testing.T, trial int, start, end int64, want, got WindowResult) {
	t.Helper()
	if len(want.Stages) != len(got.Stages) {
		t.Fatalf("trial %d window [%d,%d): want %d stages, got %d", trial, start, end, len(want.Stages), len(got.Stages))
	}
	for i := range want.Stages {
		w, g := want.Stages[i], got.Stages[i]
		if w.Stage != g.Stage || w.Samples != g.Samples || w.Average != g.Average || w.Median != g.Median {
			t.Fatalf("trial %d window [%d,%d) stage %d: want %+v, got %+v", trial, start, end, i, w, g)
		}
	}
	if len(want.Bottleneck) != len(got.Bottleneck) {
		t.Fatalf("trial %d window [%d,%d): want bottleneck %v, got %v", trial, start, end, want.Bottleneck, got.Bottleneck)
	}
	for i := range want.Bottleneck {
		if want.Bottleneck[i] != got.Bottleneck[i] {
			t.Fatalf("trial %d window [%d,%d): want bottleneck %v, got %v", trial, start, end, want.Bottleneck, got.Bottleneck)
		}
	}
}
