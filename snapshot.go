package bottleneck

import (
	"fmt"
	"sort"
)

// stagePair 是构建期的临时项：某份提交在某个阶段的汇总样本。
type stagePair struct {
	time  int64 // 提交最终完成时间
	value int64 // 该提交在此阶段所有轮次停留秒数之和
}

// stageData 保存单个阶段的只读统计数据。
// times 与 values 按完成时间升序对齐；levels 是建在 values 上的归并排序树，
// levels[i] 中从下标 j<<i 开始、长度 min(1<<i, n-j<<i) 的段是有序的。
type stageData struct {
	times  []int64
	values []int64
	prefix []int64 // values 的前缀和，长度 n+1
	levels [][]int64
}

// Snapshot 是台账的只读统计快照，建立后可反复查询。
type Snapshot struct {
	stageCount int
	stages     []stageData
}

// BuildSnapshot 由输入台账建立只读统计快照。
// stageCount 为表单阶段数，取值 [1, MaxStages]。
// 仅 StatusApproved 与 StatusRejected 的提交参与统计；
// 每份提交在每个实际到达的阶段贡献一个样本，样本值为各轮次停留秒数之和。
func BuildSnapshot(submissions []Submission, stageCount int) (*Snapshot, error) {
	if stageCount < 1 || stageCount > MaxStages {
		return nil, fmt.Errorf("bottleneck: stageCount %d out of range [1, %d]", stageCount, MaxStages)
	}

	pairs := make([][]stagePair, stageCount)
	for i := range submissions {
		sub := &submissions[i]
		if sub.Status != StatusApproved && sub.Status != StatusRejected {
			continue
		}
		var sums [MaxStages]int64
		var reached [MaxStages]bool
		for _, st := range sub.Stays {
			if st.Stage < 0 || st.Stage >= stageCount {
				return nil, fmt.Errorf("bottleneck: submission %q has stage %d out of range [0, %d)", sub.ID, st.Stage, stageCount)
			}
			if st.Leave < st.Enter {
				return nil, fmt.Errorf("bottleneck: submission %q has leave %d before enter %d at stage %d", sub.ID, st.Leave, st.Enter, st.Stage)
			}
			sums[st.Stage] += st.Leave - st.Enter
			reached[st.Stage] = true
		}
		for stage := 0; stage < stageCount; stage++ {
			if reached[stage] {
				pairs[stage] = append(pairs[stage], stagePair{time: sub.CompletedAt, value: sums[stage]})
			}
		}
	}

	snap := &Snapshot{stageCount: stageCount, stages: make([]stageData, stageCount)}
	for stage := 0; stage < stageCount; stage++ {
		snap.stages[stage] = buildStageData(pairs[stage])
	}
	return snap, nil
}

// buildStageData 将一个阶段的样本对整理为只读统计结构。
func buildStageData(ps []stagePair) stageData {
	sort.Slice(ps, func(i, j int) bool { return ps[i].time < ps[j].time })

	n := len(ps)
	d := stageData{
		times:  make([]int64, n),
		values: make([]int64, n),
		prefix: make([]int64, n+1),
	}
	for i, p := range ps {
		d.times[i] = p.time
		d.values[i] = p.value
		d.prefix[i+1] = d.prefix[i] + p.value
	}

	d.levels = append(d.levels, d.values)
	for size := 2; ; size <<= 1 {
		prev := d.levels[len(d.levels)-1]
		cur := make([]int64, n)
		half := size >> 1
		for start := 0; start < n; start += size {
			mid := start + half
			if mid > n {
				mid = n
			}
			end := start + size
			if end > n {
				end = n
			}
			mergeSorted(cur[start:end], prev[start:mid], prev[mid:end])
		}
		d.levels = append(d.levels, cur)
		if size >= n {
			break
		}
	}
	return d
}

// mergeSorted 将两个有序切片归并到 dst，len(dst) == len(a)+len(b)。
func mergeSorted(dst, a, b []int64) {
	i, j := 0, 0
	for k := range dst {
		if j >= len(b) || (i < len(a) && a[i] <= b[j]) {
			dst[k] = a[i]
			i++
		} else {
			dst[k] = b[j]
			j++
		}
	}
}
