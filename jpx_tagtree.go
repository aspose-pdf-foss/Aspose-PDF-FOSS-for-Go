// SPDX-License-Identifier: MIT

package asposepdf

// JPEG2000 tag trees (ISO/IEC 15444-1 §B.10.2) used in packet headers for
// code-block inclusion and zero-bit-plane signalling. Faithful port of pdf.js.

// jpxLog2Floor matches pdf.js's log2 = ceil(log2(x)) for x>0 (0 for x<=0):
// the smallest n>=0 with 2^n >= x. log2(1)=0, log2(3)=2, log2(5)=3 — equal to
// floor only for powers of two. The tag-tree level count depends on this exact
// convention, so a non-power-of-two code-block grid gets the right tree depth.
func jpxLog2Floor(x int) int {
	if x <= 1 {
		return 0
	}
	n, v := 0, 1
	for v < x {
		v <<= 1
		n++
	}
	return n
}

type jpxTagLevel struct {
	width, height int
	items         map[int]int
	index         int
}

type jpxTagTree struct {
	levels       []*jpxTagLevel
	currentLevel int
	value        int
}

func newJPXTagTree(width, height int) *jpxTagTree {
	t := &jpxTagTree{}
	n := jpxLog2Floor(maxI(width, height)) + 1
	for i := 0; i < n; i++ {
		t.levels = append(t.levels, &jpxTagLevel{width: width, height: height, items: map[int]int{}})
		width = ceilDivJ(width, 2)
		height = ceilDivJ(height, 2)
	}
	return t
}

func (t *jpxTagTree) reset(i, j int) {
	currentLevel := 0
	value := 0
	for currentLevel < len(t.levels) {
		level := t.levels[currentLevel]
		index := i + j*level.width
		if v, ok := level.items[index]; ok {
			value = v
			break
		}
		level.index = index
		i >>= 1
		j >>= 1
		currentLevel++
	}
	currentLevel--
	level := t.levels[currentLevel]
	level.items[level.index] = value
	t.currentLevel = currentLevel
}

func (t *jpxTagTree) incrementValue() {
	level := t.levels[t.currentLevel]
	level.items[level.index]++
}

func (t *jpxTagTree) nextLevel() bool {
	currentLevel := t.currentLevel
	level := t.levels[currentLevel]
	value := level.items[level.index]
	currentLevel--
	if currentLevel < 0 {
		t.value = value
		return false
	}
	t.currentLevel = currentLevel
	level = t.levels[currentLevel]
	level.items[level.index] = value
	return true
}

type jpxInclLevel struct {
	width, height int
	items         []int
	index         int
}

type jpxInclusionTree struct {
	levels       []*jpxInclLevel
	currentLevel int
}

func newJPXInclusionTree(width, height, defaultValue int) *jpxInclusionTree {
	t := &jpxInclusionTree{}
	n := jpxLog2Floor(maxI(width, height)) + 1
	for i := 0; i < n; i++ {
		items := make([]int, width*height)
		for k := range items {
			items[k] = defaultValue
		}
		t.levels = append(t.levels, &jpxInclLevel{width: width, height: height, items: items})
		width = ceilDivJ(width, 2)
		height = ceilDivJ(height, 2)
	}
	return t
}

func (t *jpxInclusionTree) reset(i, j, stopValue int) bool {
	currentLevel := 0
	for currentLevel < len(t.levels) {
		level := t.levels[currentLevel]
		index := i + j*level.width
		level.index = index
		value := level.items[index]
		if value == 0xff {
			break
		}
		if value > stopValue {
			t.currentLevel = currentLevel
			t.propagateValues()
			return false
		}
		i >>= 1
		j >>= 1
		currentLevel++
	}
	t.currentLevel = currentLevel - 1
	return true
}

func (t *jpxInclusionTree) incrementValue(stopValue int) {
	level := t.levels[t.currentLevel]
	level.items[level.index] = stopValue + 1
	t.propagateValues()
}

func (t *jpxInclusionTree) propagateValues() {
	levelIndex := t.currentLevel
	level := t.levels[levelIndex]
	currentValue := level.items[level.index]
	for levelIndex--; levelIndex >= 0; levelIndex-- {
		level = t.levels[levelIndex]
		level.items[level.index] = currentValue
	}
}

func (t *jpxInclusionTree) nextLevel() bool {
	currentLevel := t.currentLevel
	level := t.levels[currentLevel]
	value := level.items[level.index]
	level.items[level.index] = 0xff
	currentLevel--
	if currentLevel < 0 {
		return false
	}
	t.currentLevel = currentLevel
	level = t.levels[currentLevel]
	level.items[level.index] = value
	return true
}
