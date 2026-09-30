package bottleneck

import (
	"fmt"
	"math/rand"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"
)

// TestPerformance 按非功能要求验收：
// 20 万提交、合计 200 万阶段停留、10 个阶段；
// 构建 <= 30s，1 万次窗口查询 <= 10s，进程峰值内存 <= 768 MiB。
func TestPerformance(t *testing.T) {
	const (
		numSubs   = 200_000
		numStages = 10
		numStays  = 2_000_000
		numQuery  = 10_000
	)
	stages := make([]string, numStages)
	for i := range stages {
		stages[i] = fmt.Sprintf("stage-%d", i)
	}

	rng := rand.New(rand.NewSource(1))
	const horizon = 365 * 24 * 3600
	subs := make([]Submission, 0, numSubs)
	staysPerSub := numStays / numSubs // 10 条/提交，覆盖全部 10 个阶段
	for i := 0; i < numSubs; i++ {
		status := StatusApproved
		if i%3 == 0 {
			status = StatusRejected
		}
		sub := Submission{
			ID:          fmt.Sprintf("sub-%d", i),
			Status:      status,
			CompletedAt: int64(rng.Intn(horizon)),
		}
		enter := int64(0)
		for k := 0; k < staysPerSub; k++ {
			dwell := int64(rng.Intn(3600*24) + 1)
			sub.Stays = append(sub.Stays, Stay{
				Stage:   stages[k%numStages],
				EnterAt: enter,
				LeaveAt: enter + dwell,
			})
			enter += dwell
		}
		subs = append(subs, sub)
	}

	start := time.Now()
	snap := BuildSnapshot("v-perf", stages, subs)
	buildDur := time.Since(start)

	// 生成 1 万个窗口：一半覆盖 20%~80% 的提交，其余为随机窄窗口。
	times := make([]int64, 0, numSubs)
	for _, s := range subs {
		times = append(times, s.CompletedAt)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	type window struct{ lo, hi int64 }
	windows := make([]window, 0, numQuery)
	for i := 0; i < numQuery/2; i++ {
		cov := 0.2 + 0.6*rng.Float64()
		loQ := rng.Float64() * (1 - cov)
		lo := times[int(loQ*float64(numSubs))]
		hi := times[int((loQ+cov)*float64(numSubs))-1] + 1
		windows = append(windows, window{lo, hi})
	}
	for i := 0; i < numQuery/2; i++ {
		lo := int64(rng.Intn(horizon))
		hi := lo + int64(rng.Intn(3600*24*30)) // 最宽 30 天
		windows = append(windows, window{lo, hi})
	}

	qstart := time.Now()
	var totalSamples int
	for _, w := range windows {
		res := snap.Query(w.lo, w.hi)
		for _, st := range res.Stages {
			totalSamples += st.Samples
		}
	}
	queryDur := time.Since(qstart)

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	peakMiB := float64(ru.Maxrss) / (1 << 20) // macOS: 字节

	t.Logf("构建耗时: %v (限 30s)", buildDur)
	t.Logf("1 万次查询总耗时: %v (限 10s)，命中样本合计 %d", queryDur, totalSamples)
	t.Logf("进程峰值内存: %.1f MiB (限 768 MiB)，堆分配 %.1f MiB",
		peakMiB, float64(ms.TotalAlloc)/(1<<20))

	if buildDur > 30*time.Second {
		t.Fatalf("构建超时: %v", buildDur)
	}
	if queryDur > 10*time.Second {
		t.Fatalf("查询超时: %v", queryDur)
	}
	if peakMiB > 768 {
		t.Fatalf("峰值内存超限: %.1f MiB", peakMiB)
	}
	if totalSamples == 0 {
		t.Fatal("查询未命中任何样本")
	}
}
