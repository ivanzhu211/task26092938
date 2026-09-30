package bottleneck

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

var testStages = []string{"A", "B", "C"}

// 验收用例：一份提交在 A 停留 2s 和 8s，另两份分别 4s、6s。
func TestAcceptanceStageAggregation(t *testing.T) {
	subs := []Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 100, Stays: []Stay{
			{Stage: "A", EnterAt: 10, LeaveAt: 12},
			{Stage: "A", EnterAt: 50, LeaveAt: 58},
		}},
		{ID: "s2", Status: StatusRejected, CompletedAt: 101, Stays: []Stay{
			{Stage: "A", EnterAt: 10, LeaveAt: 14},
		}},
		{ID: "s3", Status: StatusApproved, CompletedAt: 102, Stays: []Stay{
			{Stage: "A", EnterAt: 10, LeaveAt: 16},
		}},
	}
	snap := BuildSnapshot("v1", testStages, subs)
	res := snap.Query(0, 200)
	if len(res.Stages) != 1 {
		t.Fatalf("stages=%v, want only A", res.Stages)
	}
	st := res.Stages[0]
	if st.Stage != "A" || st.Samples != 3 {
		t.Fatalf("got %+v", st)
	}
	if want := 20.0 / 3.0; st.Average != want {
		t.Fatalf("average=%v want %v", st.Average, want)
	}
	if st.Median != 6 {
		t.Fatalf("median=%v want 6", st.Median)
	}
	if len(res.Bottlenecks) != 1 || res.Bottlenecks[0] != "A" {
		t.Fatalf("bottlenecks=%v", res.Bottlenecks)
	}
}

// 完成时间落在相邻窗口公共边界：只属于后一个窗口；
// 窗口开始前的阶段停留仍完整计入。
func TestWindowBoundary(t *testing.T) {
	subs := []Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 1000, Stays: []Stay{
			{Stage: "A", EnterAt: 100, LeaveAt: 105}, // 远在窗口开始之前
		}},
		{ID: "s2", Status: StatusApproved, CompletedAt: 999, Stays: []Stay{
			{Stage: "A", EnterAt: 100, LeaveAt: 103},
		}},
	}
	snap := BuildSnapshot("v1", testStages, subs)

	left := snap.Query(0, 1000)
	if len(left.Stages) != 1 || left.Stages[0].Samples != 1 || left.Stages[0].Median != 3 {
		t.Fatalf("left window: %+v", left.Stages)
	}
	right := snap.Query(1000, 2000)
	if len(right.Stages) != 1 || right.Stages[0].Samples != 1 || right.Stages[0].Median != 5 {
		t.Fatalf("right window: %+v", right.Stages)
	}
}

// 草稿、待审批、退回修改整体排除，即使有关闭的阶段记录。
func TestStatusFiltering(t *testing.T) {
	stays := []Stay{{Stage: "A", EnterAt: 0, LeaveAt: 10}}
	subs := []Submission{
		{ID: "d", Status: StatusDraft, CompletedAt: 100, Stays: stays},
		{ID: "p", Status: StatusPending, CompletedAt: 100, Stays: stays},
		{ID: "r", Status: StatusReturned, CompletedAt: 100, Stays: stays},
	}
	snap := BuildSnapshot("v1", testStages, subs)
	res := snap.Query(0, 200)
	if len(res.Stages) != 0 || len(res.Bottlenecks) != 0 {
		t.Fatalf("want empty, got %+v", res)
	}
}

// 实际到达但停留 0 秒仍贡献样本；未到达的阶段不出现在结果中。
func TestZeroDwellAndUnreached(t *testing.T) {
	subs := []Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 100, Stays: []Stay{
			{Stage: "A", EnterAt: 10, LeaveAt: 10},
		}},
		{ID: "s2", Status: StatusApproved, CompletedAt: 101, Stays: []Stay{
			{Stage: "A", EnterAt: 10, LeaveAt: 18},
			{Stage: "B", EnterAt: 20, LeaveAt: 24},
		}},
	}
	snap := BuildSnapshot("v1", testStages, subs)
	res := snap.Query(0, 200)
	if len(res.Stages) != 2 {
		t.Fatalf("stages=%+v", res.Stages)
	}
	if res.Stages[0].Stage != "A" || res.Stages[0].Samples != 2 || res.Stages[0].Median != 4 {
		t.Fatalf("A: %+v", res.Stages[0])
	}
	if res.Stages[1].Stage != "B" || res.Stages[1].Samples != 1 {
		t.Fatalf("B: %+v", res.Stages[1])
	}
}

// 偶数样本中位数为中央两值的算术平均（含 .5）。
func TestEvenMedian(t *testing.T) {
	subs := []Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 1, Stays: []Stay{{Stage: "A", EnterAt: 0, LeaveAt: 3}}},
		{ID: "s2", Status: StatusApproved, CompletedAt: 2, Stays: []Stay{{Stage: "A", EnterAt: 0, LeaveAt: 4}}},
	}
	snap := BuildSnapshot("v1", testStages, subs)
	res := snap.Query(0, 10)
	if res.Stages[0].Median != 3.5 {
		t.Fatalf("median=%v want 3.5", res.Stages[0].Median)
	}
}

// 中位数并列时瓶颈全部返回。
func TestBottleneckTie(t *testing.T) {
	subs := []Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 1, Stays: []Stay{
			{Stage: "A", EnterAt: 0, LeaveAt: 5},
			{Stage: "B", EnterAt: 0, LeaveAt: 5},
			{Stage: "C", EnterAt: 0, LeaveAt: 2},
		}},
	}
	snap := BuildSnapshot("v1", testStages, subs)
	res := snap.Query(0, 10)
	if len(res.Bottlenecks) != 2 || res.Bottlenecks[0] != "A" || res.Bottlenecks[1] != "B" {
		t.Fatalf("bottlenecks=%v", res.Bottlenecks)
	}
}

// 空窗口：无合格提交时结果为空。
func TestEmptyWindow(t *testing.T) {
	subs := []Submission{
		{ID: "s1", Status: StatusApproved, CompletedAt: 100, Stays: []Stay{{Stage: "A", EnterAt: 0, LeaveAt: 5}}},
	}
	snap := BuildSnapshot("v1", testStages, subs)
	res := snap.Query(0, 50)
	if len(res.Stages) != 0 || len(res.Bottlenecks) != 0 {
		t.Fatalf("want empty, got %+v", res)
	}
}

// 随机小规模历史与独立暴力计算对拍。
func TestRandomCrossCheck(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	stages := []string{"A", "B", "C", "D"}
	for trial := 0; trial < 100; trial++ {
		n := 1 + rng.Intn(30)
		subs := make([]Submission, 0, n)
		for i := 0; i < n; i++ {
			sub := Submission{
				ID:          string(rune('a' + i)),
				Status:      Status(rng.Intn(5)),
				CompletedAt: int64(rng.Intn(500)),
			}
			for _, st := range stages {
				if rng.Intn(2) == 0 {
					continue // 条件审批跳过该阶段
				}
				rounds := 1 + rng.Intn(3) // 退回重提多轮
				for k := 0; k < rounds; k++ {
					enter := int64(rng.Intn(400))
					sub.Stays = append(sub.Stays, Stay{
						Stage:   st,
						EnterAt: enter,
						LeaveAt: enter + int64(rng.Intn(20)),
					})
				}
			}
			subs = append(subs, sub)
		}
		snap := BuildSnapshot("v1", stages, subs)
		for q := 0; q < 20; q++ {
			start := int64(rng.Intn(500))
			end := start + int64(rng.Intn(200))
			got := snap.Query(start, end)
			want := bruteQuery(subs, stages, start, end)
			compareResult(t, got, want)
		}
	}
}

// bruteQuery 独立暴力实现，用于对拍。
func bruteQuery(subs []Submission, stages []string, start, end int64) WindowResult {
	var res WindowResult
	var maxMedian float64
	for _, st := range stages {
		var dwells []int64
		for _, sub := range subs {
			if sub.Status != StatusApproved && sub.Status != StatusRejected {
				continue
			}
			if sub.CompletedAt < start || sub.CompletedAt >= end {
				continue
			}
			var total int64
			reached := false
			for _, stay := range sub.Stays {
				if stay.Stage == st {
					total += stay.LeaveAt - stay.EnterAt
					reached = true
				}
			}
			if reached {
				dwells = append(dwells, total)
			}
		}
		if len(dwells) == 0 {
			continue
		}
		sort.Slice(dwells, func(i, j int) bool { return dwells[i] < dwells[j] })
		var sum int64
		for _, d := range dwells {
			sum += d
		}
		var median float64
		m := len(dwells)
		if m%2 == 1 {
			median = float64(dwells[m/2])
		} else {
			median = float64(dwells[m/2-1])/2 + float64(dwells[m/2])/2
		}
		if len(res.Stages) == 0 || median > maxMedian {
			maxMedian = median
		}
		res.Stages = append(res.Stages, StageStats{
			Stage:   st,
			Samples: len(dwells),
			Average: float64(sum) / float64(m),
			Median:  median,
		})
	}
	for _, s := range res.Stages {
		if s.Median == maxMedian {
			res.Bottlenecks = append(res.Bottlenecks, s.Stage)
		}
	}
	return res
}

func compareResult(t *testing.T, got, want WindowResult) {
	t.Helper()
	if len(got.Stages) != len(want.Stages) {
		t.Fatalf("stage count got %d want %d\ngot=%+v\nwant=%+v", len(got.Stages), len(want.Stages), got, want)
	}
	for i := range got.Stages {
		g, w := got.Stages[i], want.Stages[i]
		if g.Stage != w.Stage || g.Samples != w.Samples ||
			math.Abs(g.Average-w.Average) > 1e-12 || g.Median != w.Median {
			t.Fatalf("stage %d got %+v want %+v", i, g, w)
		}
	}
	if len(got.Bottlenecks) != len(want.Bottlenecks) {
		t.Fatalf("bottlenecks got %v want %v", got.Bottlenecks, want.Bottlenecks)
	}
	for i := range got.Bottlenecks {
		if got.Bottlenecks[i] != want.Bottlenecks[i] {
			t.Fatalf("bottlenecks got %v want %v", got.Bottlenecks, want.Bottlenecks)
		}
	}
}
