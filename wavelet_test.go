package bottleneck

import (
	"math/rand"
	"sort"
	"testing"
)

func kthBrute(values []int64, l, r, k int) int64 {
	tmp := append([]int64(nil), values[l:r]...)
	sort.Slice(tmp, func(i, j int) bool { return tmp[i] < tmp[j] })
	return tmp[k]
}

func TestWaveletKthRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 50; trial++ {
		n := 1 + rng.Intn(300)
		values := make([]int64, n)
		set := map[int64]bool{}
		for i := range values {
			v := int64(rng.Intn(1000)) - 500
			values[i] = v
			set[v] = true
		}
		uniqs := make([]int64, 0, len(set))
		for v := range set {
			uniqs = append(uniqs, v)
		}
		sort.Slice(uniqs, func(i, j int) bool { return uniqs[i] < uniqs[j] })
		wt := newWaveletTree(values, uniqs)
		for q := 0; q < 200; q++ {
			l := rng.Intn(n)
			r := l + 1 + rng.Intn(n-l)
			k := rng.Intn(r - l)
			got := wt.kth(l, r, k)
			want := kthBrute(values, l, r, k)
			if got != want {
				t.Fatalf("kth(%d,%d,%d)=%d want %d", l, r, k, got, want)
			}
		}
	}
}

func TestWaveletKthSingleValue(t *testing.T) {
	values := []int64{7, 7, 7}
	wt := newWaveletTree(values, []int64{7})
	if got := wt.kth(0, 3, 2); got != 7 {
		t.Fatalf("got %d want 7", got)
	}
}
