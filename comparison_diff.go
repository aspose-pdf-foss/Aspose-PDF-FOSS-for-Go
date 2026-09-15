// SPDX-License-Identifier: MIT

package asposepdf

import "strings"

// maxEditDistance caps the Myers search. Two unrelated documents drive the
// edit distance towards the sum of their lengths, and the search cost grows
// with the square of that distance in both time and memory — myersScript
// snapshots the whole frontier once per round, so the cap bounds a roughly
// maxEditDistance^2-int allocation (about 64 MB at this value). A typical
// comparison has a small edit distance and allocates kilobytes, since the
// common prefix and suffix are trimmed before the search starts. Past the
// cap, the comparer reports one wholesale deletion and one wholesale
// insertion instead of grinding through — and allocating — the full square.
const maxEditDistance = 2000

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

	ta, tb := a[pre:len(a)-suf], b[pre:len(b)-suf]

	// Once either trimmed side is empty, the answer is already known: the
	// other side is one run of deletions or insertions. Skipping the Myers
	// search here matters — a page-sized deletion would otherwise pay the
	// full O(D^2) cost (frontier snapshots up to maxEditDistance) on every
	// page just to rediscover that there was nothing to search for.
	var mid []edit
	ok := true
	switch {
	case len(ta) == 0:
		mid = make([]edit, len(tb))
		for i := range tb {
			mid[i] = edit{kind: editInsert, a: -1, b: i}
		}
	case len(tb) == 0:
		mid = make([]edit, len(ta))
		for i := range ta {
			mid[i] = edit{kind: editDelete, a: i, b: -1}
		}
	default:
		mid, ok = myersScript(ta, tb)
	}
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

// groupOperations turns an edit script into runs. Adjacent edits of the same
// kind merge while they stay on the same page; their rectangles are unioned
// per line, so a run crossing two lines reports two boxes.
func groupOperations(edits []edit, src, dst []wordToken, order EditOperationsOrder) []DiffOperation {
	var ops []DiffOperation
	for i := 0; i < len(edits); {
		j := i + 1
		for j < len(edits) && edits[j].kind == edits[i].kind &&
			samePage(edits[i], edits[j], src, dst) {
			j++
		}
		ops = append(ops, buildOperation(edits[i:j], src, dst))
		i = j
	}
	if order == EditOperationsInsertFirst {
		swapReplacements(ops)
	}
	return ops
}

// samePage reports whether two edits of the same kind sit on the same page of
// the side they belong to.
func samePage(a, b edit, src, dst []wordToken) bool {
	switch a.kind {
	case editInsert:
		return dst[a.b].page == dst[b.b].page
	case editDelete:
		return src[a.a].page == src[b.a].page
	default:
		return src[a.a].page == src[b.a].page && dst[a.b].page == dst[b.b].page
	}
}

// buildOperation assembles one run into a DiffOperation.
func buildOperation(run []edit, src, dst []wordToken) DiffOperation {
	op := DiffOperation{}
	var srcTokens, dstTokens []wordToken
	for _, e := range run {
		if e.a >= 0 {
			srcTokens = append(srcTokens, src[e.a])
		}
		if e.b >= 0 {
			dstTokens = append(dstTokens, dst[e.b])
		}
	}
	switch run[0].kind {
	case editInsert:
		op.Operation = OperationInsert
		op.Text = joinTokenText(dstTokens)
	case editDelete:
		op.Operation = OperationDelete
		op.Text = joinTokenText(srcTokens)
	default:
		op.Operation = OperationEqual
		op.Text = joinTokenText(srcTokens)
	}
	if len(srcTokens) > 0 {
		op.SourcePage = srcTokens[0].page
		op.SourceRects = lineRects(srcTokens)
	}
	if len(dstTokens) > 0 {
		op.DestPage = dstTokens[0].page
		op.DestRects = lineRects(dstTokens)
	}
	return op
}

// joinTokenText joins the words of a run with single spaces.
func joinTokenText(tokens []wordToken) string {
	parts := make([]string, len(tokens))
	for i, tk := range tokens {
		parts[i] = tk.text
	}
	return strings.Join(parts, " ")
}

// lineRects unions the tokens' rectangles per layout line, keeping document
// order, so a run wrapping onto the next line reports one box per line.
// Precondition: tokens arrive in non-decreasing line order — true by
// construction, since tokens are produced in reading order.
func lineRects(tokens []wordToken) []Rectangle {
	var (
		rects []Rectangle
		cur   Rectangle
		line  = -1
		open  bool
	)
	for _, tk := range tokens {
		if !open || tk.line != line {
			if open {
				rects = append(rects, cur)
			}
			cur, line, open = tk.rect, tk.line, true
			continue
		}
		cur = unionRect(cur, tk.rect)
	}
	if open {
		rects = append(rects, cur)
	}
	return rects
}

// swapReplacements puts the inserted half of a replacement before the deleted
// half, for EditOperationsInsertFirst. It only reorders the pair at the
// adjacent delete/insert junction: a replacement split across a page
// boundary (the delete on one page, the insert on the next) comes out
// interleaved rather than with every insert first.
func swapReplacements(ops []DiffOperation) {
	for i := 0; i+1 < len(ops); i++ {
		if ops[i].Operation == OperationDelete && ops[i+1].Operation == OperationInsert {
			ops[i], ops[i+1] = ops[i+1], ops[i]
			i++ // the pair is settled; do not reconsider it
		}
	}
}

// wholeReplacement is the answer when the edit-distance cap is hit: the whole
// source is reported deleted and the whole destination inserted, page by page
// so the rectangles stay usable.
func wholeReplacement(src, dst []wordToken, order EditOperationsOrder) []DiffOperation {
	edits := make([]edit, 0, len(src)+len(dst))
	for i := range src {
		edits = append(edits, edit{kind: editDelete, a: i, b: -1})
	}
	for i := range dst {
		edits = append(edits, edit{kind: editInsert, a: -1, b: i})
	}
	return groupOperations(edits, src, dst, order)
}
