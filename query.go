package bottleneck

import "sort"

// Query 按提交最终完成时间窗口 [start, end) 查询统计结果。
// 窗口包含起点、不包含终点；选中提交的全部阶段历史都参与计算。
// 窗口内无合格提交时，Stages 与 Bottleneck 均为空。
func (s *Snapshot) Query(start, end int64) WindowResult {
	res := WindowResult{}
	var maxMedian float64
	for stage := 0; stage < s.stageCount; stage++ {
		d := &s.stages[stage]
		l := sort.Search(len(d.times), func(i int) bool { return d.times[i] >= start })
		r := sort.Search(len(d.times), func(i int) bool { return d.times[i] >= end })
		n := r - l
		if n <= 0 {
			continue
		}
		sum := d.prefix[r] - d.prefix[l]
		lo := d.kth(l, r, (n-1)/2)
		hi := d.kth(l, r, n/2)
		stats := StageStats{
			Stage:   stage,
			Samples: n,
			Average: float64(sum) / float64(n),
			Median:  float64(lo)/2 + float64(hi)/2,
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

// kth 返回 values[l:r] 中第 k 小的值（k 从 0 开始）。
func (d *stageData) kth(l, r, k int) int64 {
	root := d.levels[len(d.levels)-1]
	idx := sort.Search(len(root), func(i int) bool {
		return d.countLE(l, r, root[i]) > k
	})
	return root[idx]
}

// countLE 统计 values[l:r] 中不大于 v 的元素个数。
func (d *stageData) countLE(l, r int, v int64) int {
	count := 0
	n := len(d.values)
	level := 0
	for l < r {
		if l&1 == 1 {
			count += d.runCountLE(level, l, n, v)
			l++
		}
		if r&1 == 1 {
			r--
			count += d.runCountLE(level, r, n, v)
		}
		l >>= 1
		r >>= 1
		level++
	}
	return count
}

// runCountLE 统计第 level 层第 j 个有序段中不大于 v 的元素个数。
func (d *stageData) runCountLE(level, j, n int, v int64) int {
	start := j << level
	end := start + 1<<level
	if end > n {
		end = n
	}
	run := d.levels[level][start:end]
	return sort.Search(len(run), func(i int) bool { return run[i] > v })
}
