// SPDX-License-Identifier: MIT

package asposepdf

// maxEditDistance caps the Myers search. Two unrelated documents drive the
// edit distance towards the sum of their lengths, and the search cost grows
// with its square; past this point the comparer reports one wholesale
// deletion and one wholesale insertion instead of grinding for minutes.
const maxEditDistance = 4096

type editKind int

const (
	editEqual editKind = iota
	editInsert
	editDelete
)

// edit is one step of the script turning the source sequence into the
// destination one. a indexes the source (-1 for an insertion), b the
// destination (-1 for a deletion).
type edit struct {
	kind editKind
	a, b int
}

// diffKeys returns the shortest edit script turning a into b, comparing the
// strings for equality. The second result is false when the search exceeded
// maxEditDistance, in which case no script is returned.
func diffKeys(a, b []string) ([]edit, bool) {
	// Common prefix and suffix are equal by construction; trimming them keeps
	// the quadratic part of the search proportional to what actually changed.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre &&
		a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}

	mid, ok := myersScript(a[pre:len(a)-suf], b[pre:len(b)-suf])
	if !ok {
		return nil, false
	}

	out := make([]edit, 0, pre+len(mid)+suf)
	for i := 0; i < pre; i++ {
		out = append(out, edit{kind: editEqual, a: i, b: i})
	}
	for _, e := range mid {
		shifted := e
		if shifted.a >= 0 {
			shifted.a += pre
		}
		if shifted.b >= 0 {
			shifted.b += pre
		}
		out = append(out, shifted)
	}
	for i := 0; i < suf; i++ {
		out = append(out, edit{kind: editEqual, a: len(a) - suf + i, b: len(b) - suf + i})
	}
	return out, true
}

// myersScript is the greedy O(ND) algorithm of Myers (1986): it advances a
// furthest-reaching frontier one edit at a time, snapshotting the frontier so
// the script can be walked back once the end is reached.
func myersScript(a, b []string) ([]edit, bool) {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil, true
	}
	maxD := n + m
	if maxD > maxEditDistance {
		maxD = maxEditDistance
	}
	offset := maxD
	v := make([]int, 2*maxD+1)
	trace := make([][]int, 0, maxD+1)

	for d := 0; d <= maxD; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)

		for k := -d; k <= d; k += 2 {
			var x int
			switch {
			case k == -d:
				x = v[offset+k+1]
			case k != d && v[offset+k-1] < v[offset+k+1]:
				x = v[offset+k+1]
			default:
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrackScript(trace, offset, n, m), true
			}
		}
	}
	return nil, false
}

// backtrackScript walks the saved frontiers from the end back to the origin,
// emitting the diagonal (equal) moves and the single edit of each step.
func backtrackScript(trace [][]int, offset, n, m int) []edit {
	rev := make([]edit, 0, n+m)
	x, y := n, m
	for d := len(trace) - 1; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		switch {
		case k == -d:
			prevK = k + 1
		case k != d && v[offset+k-1] < v[offset+k+1]:
			prevK = k + 1
		default:
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, edit{kind: editEqual, a: x, b: y})
		}
		if prevK == k+1 {
			y--
			rev = append(rev, edit{kind: editInsert, a: -1, b: y})
		} else {
			x--
			rev = append(rev, edit{kind: editDelete, a: x, b: -1})
		}
		x, y = prevX, prevY
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, edit{kind: editEqual, a: x, b: y})
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}
