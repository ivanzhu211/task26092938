package bottleneck

import (
	"fmt"
	"math/rand"
	"syscall"
	"testing"
	"time"
)

// 规模验收：20万份提交、合计200万次阶段停留、10个阶段；
// 构建不超过30秒，1万次不同窗口查询总耗时不超过10秒，
// 包含输入台账在内的进程峰值内存不超过768 MiB。
func TestPerformanceAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping scale test in short mode")
	}
	const (
		numSubs   = 200_000
		numStages = 10
		numStays  = 2_000_000 // 每份提交每阶段1次停留
		numQuery  = 10_000
	)

	rng := rand.New(rand.NewSource(7))
	const span = int64(2 * 365 * 24 * 3600) // 完成时间跨度约两年
	base := int64(1_700_000_000)

	subs := make([]Submission, 0, numSubs)
	stayCount := 0
	for i := 0; i < numSubs; i++ {
		sub := Submission{
			ID:          fmt.Sprintf("sub-%d", i),
			Status:      StatusApproved + Status(rng.Intn(2)), // 批准/拒绝各半
			CompletedAt: base + int64(rng.Int63n(span)),
		}
		enter := base - int64(rng.Intn(3600))
		for s := 0; s < numStages; s++ {
			dur := int64(rng.Intn(7200))
			sub.Stays = append(sub.Stays, StageStay{Stage: s, Enter: enter, Leave: enter + dur})
			enter += dur
			stayCount++
		}
		subs = append(subs, sub)
	}
	if stayCount != numStays {
		t.Fatalf("stay count: want %d, got %d", numStays, stayCount)
	}

	buildStart := time.Now()
	snap, err := BuildSnapshot(subs, numStages)
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	buildElapsed := time.Since(buildStart)

	// 生成1万个窗口：一半命中全部提交的20%~80%，其余覆盖极窄、极宽与空窗口。
	type window struct{ start, end int64 }
	windows := make([]window, 0, numQuery)
	for i := 0; i < numQuery/2; i++ {
		lo := rng.Float64() * 0.6          // 起点分位 [0, 0.6)
		hi := lo + 0.2 + rng.Float64()*0.6 // 宽度分位 [0.2, 0.8)
		if hi > 1 {
			hi = 1
		}
		windows = append(windows, window{base + int64(lo*float64(span)), base + int64(hi*float64(span))})
	}
	for i := numQuery / 2; i < numQuery; i++ {
		switch i % 3 {
		case 0: // 极窄窗口
			s := base + int64(rng.Int63n(span))
			windows = append(windows, window{s, s + 60})
		case 1: // 覆盖全程
			windows = append(windows, window{base - 1, base + span + 1})
		default: // 大概率空窗口
			s := base + span + int64(rng.Int63n(span))
			windows = append(windows, window{s, s + 3600})
		}
	}

	queryStart := time.Now()
	var totalSamples int
	for _, w := range windows {
		res := snap.Query(w.start, w.end)
		for _, st := range res.Stages {
			totalSamples += st.Samples
		}
	}
	queryElapsed := time.Since(queryStart)
	if totalSamples == 0 {
		t.Fatal("queries hit no samples")
	}

	peakBytes := maxRSSBytes()
	const (
		buildLimit = 30 * time.Second
		queryLimit = 10 * time.Second
		memLimit   = 768 * 1024 * 1024
	)
	t.Logf("build: %v (limit %v)", buildElapsed, buildLimit)
	t.Logf("query: %v total for %d windows (limit %v)", queryElapsed, numQuery, queryLimit)
	t.Logf("peak RSS: %.1f MiB (limit 768.0 MiB)", float64(peakBytes)/1024/1024)
	if buildElapsed > buildLimit {
		t.Errorf("build too slow: %v > %v", buildElapsed, buildLimit)
	}
	if queryElapsed > queryLimit {
		t.Errorf("queries too slow: %v > %v", queryElapsed, queryLimit)
	}
	if peakBytes > memLimit {
		t.Errorf("peak memory too high: %d > %d", peakBytes, memLimit)
	}
}

// maxRSSBytes 返回进程峰值常驻内存（macOS 上 ru_maxrss 单位为字节）。
func maxRSSBytes() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return ru.Maxrss
}
