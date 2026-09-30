package bottleneck

import "math/bits"

// bitVector 是支持 O(1) rank 的位向量。
// 每 512 位一个超块记录前驱 1 的个数，块内用 popcount 计算。
type bitVector struct {
	words  []uint64
	blocks []uint32 // blocks[i] = 前 i*512 位中 1 的个数
	n      int
}

func newBitVector(n int) *bitVector {
	nw := (n + 63) / 64
	nb := (n + 511) / 512
	return &bitVector{
		words:  make([]uint64, nw),
		blocks: make([]uint32, nb+1),
		n:      n,
	}
}

func (b *bitVector) set(i int) {
	b.words[i>>6] |= 1 << (uint(i) & 63)
}

// build 在所有位写入完成后构建超块前缀和。
func (b *bitVector) build() {
	var acc uint32
	for i := 0; i < b.n; i += 512 {
		b.blocks[i>>9] = acc
		hi := i + 512
		if hi > b.n {
			hi = b.n
		}
		for w := i >> 6; w < (hi+63)>>6; w++ {
			acc += uint32(bits.OnesCount64(b.words[w]))
		}
	}
	b.blocks[(b.n+511)>>9] = acc
}

// rank1 返回 [0, i) 中 1 的个数。
func (b *bitVector) rank1(i int) int {
	if i <= 0 {
		return 0
	}
	if i > b.n {
		i = b.n
	}
	base := int(b.blocks[i>>9])
	j := (i >> 9) << 3 // 超块起始字下标
	end := i >> 6
	for ; j < end; j++ {
		base += bits.OnesCount64(b.words[j])
	}
	if rem := uint(i) & 63; rem != 0 {
		base += bits.OnesCount64(b.words[end] & ((1 << rem) - 1))
	}
	return base
}

// rank0 返回 [0, i) 中 0 的个数。
func (b *bitVector) rank0(i int) int {
	return i - b.rank1(i)
}

// waveletTree 对整数序列支持区间第 k 小查询（用于精确中位数）。
// 值为压缩后的秩，uniqs 保存秩到原始值的映射。
type waveletTree struct {
	levels []*bitVector // 每层一个位向量，从最高位开始
	zeros  []int        // 每层 0 的总数
	uniqs  []int64
}

// newWaveletTree 由 values 建立小波树，uniqs 为升序去重值表。
func newWaveletTree(values []int64, uniqs []int64) *waveletTree {
	n := len(values)
	t := &waveletTree{uniqs: uniqs}
	if n == 0 {
		return t
	}
	// 压缩为秩
	ranks := make([]int, n)
	for i, v := range values {
		ranks[i] = rankOf(uniqs, v)
	}
	maxRank := len(uniqs) - 1
	depth := 1
	for (1 << uint(depth)) <= maxRank {
		depth++
	}
	t.levels = make([]*bitVector, depth)
	t.zeros = make([]int, depth)

	cur := ranks
	buf := make([]int, n)
	for d := 0; d < depth; d++ {
		shift := uint(depth - 1 - d)
		bv := newBitVector(n)
		z := 0
		for _, v := range cur {
			if (v>>shift)&1 == 0 {
				z++
			}
		}
		zi, oi := 0, z
		for i, v := range cur {
			if (v>>shift)&1 == 1 {
				bv.set(i)
				buf[oi] = v
				oi++
			} else {
				buf[zi] = v
				zi++
			}
		}
		bv.build()
		t.levels[d] = bv
		t.zeros[d] = z
		cur, buf = buf, cur
	}
	return t
}

// rankOf 二分查找 v 在升序 uniqs 中的下标。
func rankOf(uniqs []int64, v int64) int {
	lo, hi := 0, len(uniqs)-1
	for lo < hi {
		mid := (lo + hi) >> 1
		if uniqs[mid] < v {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

// kth 返回原序列 [l, r) 区间内第 k 小（k 从 0 计）的值。
func (t *waveletTree) kth(l, r, k int) int64 {
	rank := 0
	for d := 0; d < len(t.levels); d++ {
		bv := t.levels[d]
		zerosInL := bv.rank0(l)
		zerosInR := bv.rank0(r)
		if zerosInR-zerosInL > k {
			// 走 0 分支
			l, r = zerosInL, zerosInR
		} else {
			// 走 1 分支
			k -= zerosInR - zerosInL
			rank |= 1 << uint(len(t.levels)-1-d)
			z := t.zeros[d]
			l = z + bv.rank1(l)
			r = z + bv.rank1(r)
		}
	}
	return t.uniqs[rank]
}
