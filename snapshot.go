package bottleneck

import "sort"

// stageData 保存单个阶段按完成时间升序排列的样本。
type stageData struct {
	name   string
	times  []int64 // 各样本所属提交的最终完成时间，升序
	prefix []int64 // 停留秒数前缀和，长度 n+1
	tree   *waveletTree
}

// Snapshot 是只读统计快照，建立后不可变。
type Snapshot struct {
	stages []stageData
}

// BuildSnapshot 由台账建立只读统计快照。
// stages 为表单版本的阶段列表（最多 10 个）；subs 可无序。
// 只有已批准和已拒绝的提交参与统计；不属于 stages 的停留记录被忽略。
func BuildSnapshot(formVersion string, stages []string, subs []Submission) *Snapshot {
	stageIndex := make(map[string]int, len(stages))
	for i, name := range stages {
		stageIndex[name] = i
	}

	type pair struct {
		completedAt int64
		dwell       int64
	}
	perStage := make([][]pair, len(stages))

	dwells := make([]int64, len(stages))
	reached := make([]bool, len(stages))
	for _, sub := range subs {
		if sub.Status != StatusApproved && sub.Status != StatusRejected {
			continue
		}
		for i := range dwells {
			dwells[i] = 0
			reached[i] = false
		}
		for _, stay := range sub.Stays {
			idx, ok := stageIndex[stay.Stage]
			if !ok {
				continue
			}
			dwell := stay.LeaveAt - stay.EnterAt
			if dwell < 0 {
				dwell = 0
			}
			dwells[idx] += dwell
			reached[idx] = true
		}
		for i := range dwells {
			if reached[i] {
				perStage[i] = append(perStage[i], pair{sub.CompletedAt, dwells[i]})
			}
		}
	}

	snap := &Snapshot{stages: make([]stageData, len(stages))}
	for i, name := range stages {
		pairs := perStage[i]
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].completedAt < pairs[b].completedAt
		})
		n := len(pairs)
		sd := stageData{
			name:   name,
			times:  make([]int64, n),
			prefix: make([]int64, n+1),
		}
		var values []int64
		set := make(map[int64]struct{}, n)
		for j, p := range pairs {
			sd.times[j] = p.completedAt
			sd.prefix[j+1] = sd.prefix[j] + p.dwell
			set[p.dwell] = struct{}{}
		}
		if n > 0 {
			values = make([]int64, n)
			uniqs := make([]int64, 0, len(set))
			for v := range set {
				uniqs = append(uniqs, v)
			}
			sort.Slice(uniqs, func(a, b int) bool { return uniqs[a] < uniqs[b] })
			for j, p := range pairs {
				values[j] = p.dwell
			}
			sd.tree = newWaveletTree(values, uniqs)
		}
		snap.stages[i] = sd
	}
	return snap
}

// Query 查询最终完成时间落在 [start, end) 内的提交，
// 返回各阶段统计与瓶颈阶段。窗口无合格提交时结果为空。
func (s *Snapshot) Query(start, end int64) WindowResult {
	var result WindowResult
	var maxMedian float64
	for _, sd := range s.stages {
		l := lowerBound(sd.times, start)
		r := lowerBound(sd.times, end)
		count := r - l
		if count == 0 {
			continue
		}
		sum := sd.prefix[r] - sd.prefix[l]
		var median float64
		if count%2 == 1 {
			median = float64(sd.tree.kth(l, r, count/2))
		} else {
			a := sd.tree.kth(l, r, count/2-1)
			b := sd.tree.kth(l, r, count/2)
			median = float64(a)/2 + float64(b)/2
		}
		stat := StageStats{
			Stage:   sd.name,
			Samples: count,
			Average: float64(sum) / float64(count),
			Median:  median,
		}
		if len(result.Stages) == 0 || median > maxMedian {
			maxMedian = median
		}
		result.Stages = append(result.Stages, stat)
	}
	for _, stat := range result.Stages {
		if stat.Median == maxMedian {
			result.Bottlenecks = append(result.Bottlenecks, stat.Stage)
		}
	}
	return result
}

// lowerBound 返回升序数组 a 中第一个 >= v 的下标。
func lowerBound(a []int64, v int64) int {
	return sort.Search(len(a), func(i int) bool { return a[i] >= v })
}
