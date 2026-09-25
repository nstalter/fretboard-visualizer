package guitar

import "cmp"

// mutePenalty is the movement cost of a string changing between muted and played.
const mutePenalty = 2

// CalculateFingerMovement returns |Δfret| summed over strings played in both voicings,
// plus mutePenalty for each string that is played in only one of them.
func (v *GuitarChordVoicing) CalculateFingerMovement(other GuitarChordVoicing) float64 {
	total := 0
	for i := 0; i < 6; i++ {
		from, to := v.Fingering[i], other.Fingering[i]
		switch {
		case from >= 0 && to >= 0:
			total += max(from-to, to-from)
		case (from >= 0) != (to >= 0):
			total += mutePenalty
		}
	}
	return float64(total)
}

// NearestIndex returns the index of the voicing with the least finger movement from
// current. Ties go to the closer average fret (compared exactly), then the lower fret,
// then the earlier index. Returns -1 if vs is empty.
func NearestIndex(vs []GuitarChordVoicing, current [6]int) int {
	cur := GuitarChordVoicing{Fingering: current}
	sc, nc := frettedSum(current)
	best := -1
	var bestMove float64
	var bestDist, bestN int
	for i, v := range vs {
		move := cur.CalculateFingerMovement(v)
		s, n := frettedSum(v.Fingering)
		// |s/n − sc/nc| scaled by n·nc; compared across candidates by cross-multiplying n.
		dist := max(s*nc-sc*n, sc*n-s*nc)
		if best >= 0 {
			c := cmp.Compare(move, bestMove)
			if c == 0 {
				c = cmp.Compare(dist*bestN, bestDist*n)
			}
			if c == 0 {
				c = cmp.Compare(v.GetMinFret(), vs[best].GetMinFret())
			}
			if c >= 0 {
				continue
			}
		}
		best, bestMove, bestDist, bestN = i, move, dist, n
	}
	return best
}

// NearestIndexAt picks a voicing for a clicked fretboard spot: string str (internal
// high→low index) at fret. Among voicings that play exactly that spot it returns the
// nearest to current (or, when current is nil, the one closest in position to fret),
// with matched = true. When none play it, it falls back to closestToFret over all of vs
// with matched = false. Returns -1 if vs is empty.
func NearestIndexAt(vs []GuitarChordVoicing, current *[6]int, str, fret int) (index int, matched bool) {
	var hits []GuitarChordVoicing
	var hitIdx []int
	for i, v := range vs {
		if v.Fingering[str] == fret {
			hits = append(hits, v)
			hitIdx = append(hitIdx, i)
		}
	}
	if len(hits) > 0 {
		if current != nil {
			return hitIdx[NearestIndex(hits, *current)], true
		}
		return hitIdx[closestToFret(hits, nil, fret)], true
	}
	return closestToFret(vs, current, fret), false
}

// fretDistance is how far fret lies outside the voicing's played range: 0 when it is
// inside, and open strings count as fret 0.
func fretDistance(f [6]int, fret int) int {
	lo, hi := MaxFret+1, -1
	for _, x := range f {
		if x >= 0 {
			lo, hi = min(lo, x), max(hi, x)
		}
	}
	return max(lo-fret, fret-hi, 0)
}

// closestToFret returns the index of the voicing nearest a fret, ranked by how far the
// fret is from the voicing's played range (0 when it is inside), then by least movement
// from current if given, then by average fretted position (compared exactly), then by
// the earlier index. Returns -1 if vs is empty.
func closestToFret(vs []GuitarChordVoicing, current *[6]int, fret int) int {
	best := -1
	var bestRange, bestDist, bestN int
	var bestMove float64
	for i, v := range vs {
		rng := fretDistance(v.Fingering, fret)
		s, n := frettedSum(v.Fingering)
		dist := max(s-fret*n, fret*n-s) // |s/n − fret| scaled by n
		var move float64
		if current != nil {
			cur := GuitarChordVoicing{Fingering: *current}
			move = cur.CalculateFingerMovement(v)
		}
		if best >= 0 {
			c := cmp.Compare(rng, bestRange)
			if c == 0 {
				c = cmp.Compare(move, bestMove)
			}
			if c == 0 {
				c = cmp.Compare(dist*bestN, bestDist*n)
			}
			if c >= 0 {
				continue
			}
		}
		best, bestRange, bestDist, bestN, bestMove = i, rng, dist, n, move
	}
	return best
}
