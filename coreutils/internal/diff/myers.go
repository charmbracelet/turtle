package diff

// maxDiffs caps the edit distance searched by the engine. Comparisons beyond
// it fall back to one change covering everything left, keeping the worst
// case time and memory bounded on inputs too different to be worth
// minimizing.
const maxDiffs = 4096

// engine computes the changes turning one sequence of line ids into another.
type engine struct {
	changes []Change
}

// diff recurses over the line ids x and y of a sub-problem, whose first
// elements sit at lines offA and offB of the whole files.
func (e *engine) diff(x, y []int, offA, offB int) {
	for len(x) > 0 && len(y) > 0 && x[0] == y[0] {
		x, y, offA, offB = x[1:], y[1:], offA+1, offB+1
	}
	for len(x) > 0 && len(y) > 0 && x[len(x)-1] == y[len(y)-1] {
		x, y = x[:len(x)-1], y[:len(y)-1]
	}
	switch {
	case len(x) == 0 && len(y) == 0:
		return
	case len(x) == 0:
		e.add(Change{A0: offA, A1: offA, B0: offB, B1: offB + len(y)})
		return
	case len(y) == 0:
		e.add(Change{A0: offA, A1: offA + len(x), B0: offB, B1: offB})
		return
	}

	d, sx, sy, ex, ey, ok := middleSnake(x, y)
	if !ok {
		e.add(Change{A0: offA, A1: offA + len(x), B0: offB, B1: offB + len(y)})
		return
	}
	if d > 1 || sx != ex {
		e.diff(x[:sx], y[:sy], offA, offB)
		e.diff(x[ex:], y[ey:], offA+ex, offB+ey)
		return
	}
	// A single edit with no snake around it: the edit sits at the end of
	// what trimming left, as an insertion when y is longer, a deletion
	// otherwise.
	if len(y) > len(x) {
		e.add(Change{A0: offA + len(x), A1: offA + len(x), B0: offB + len(x), B1: offB + len(y)})
	} else {
		e.add(Change{A0: offA + len(y), A1: offA + len(x), B0: offB + len(y), B1: offB + len(y)})
	}
}

func (e *engine) add(c Change) {
	e.changes = append(e.changes, c)
}

// middleSnake searches the shortest edit path between x and y with the
// two-sided Myers algorithm, returning its length d and the start (sx, sy)
// and end (ex, ey) of a middle snake: a run of equal lines an optimal path
// goes through. ok is false when the search hits its limit first.
func middleSnake(x, y []int) (d, sx, sy, ex, ey int, ok bool) {
	n, m := len(x), len(y)
	maxD := min((n+m+1)/2, maxDiffs)
	delta := n - m
	off := maxD
	vf := make([]int, 2*maxD+1)
	vb := make([]int, 2*maxD+1)
	vf[1+off] = 0
	vb[1+off] = 0
	for d := range maxD + 1 {
		for k := -d; k <= d; k += 2 {
			var fx int
			if k == -d || (k != d && vf[k-1+off] < vf[k+1+off]) {
				fx = vf[k+1+off]
			} else {
				fx = vf[k-1+off] + 1
			}
			fy := fx - k
			sxi, syi := fx, fy
			for fx < n && fy < m && x[fx] == y[fy] {
				fx, fy = fx+1, fy+1
			}
			vf[k+off] = fx
			if delta&1 == 1 {
				if ik := delta - k; ik >= 1-d && ik <= d-1 && vf[k+off]+vb[ik+off] >= n {
					return 2*d - 1, sxi, syi, fx, fy, true
				}
			}
		}
		for k := -d; k <= d; k += 2 {
			var bx int
			if k == -d || (k != d && vb[k-1+off] < vb[k+1+off]) {
				bx = vb[k+1+off]
			} else {
				bx = vb[k-1+off] + 1
			}
			by := bx - k
			bxi, byi := bx, by
			for bx < n && by < m && x[n-bx-1] == y[m-by-1] {
				bx, by = bx+1, by+1
			}
			vb[k+off] = bx
			if delta&1 == 0 {
				if ik := delta - k; ik >= -d && ik <= d && vb[k+off]+vf[ik+off] >= n {
					return 2 * d, n - bx, m - by, n - bxi, m - byi, true
				}
			}
		}
	}
	return 0, 0, 0, 0, 0, false
}
