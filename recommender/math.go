package recommender

import "math"

// FeatureVector is a normalized track feature vector. Every dimension
// should be scaled to [0, 1] (or z-scored) ahead of time, since the
// normalization math below assumes bounded per-dimension range.
type FeatureVector []float64

// Dim returns the number of feature dimensions.
func (v FeatureVector) Dim() int {
	return len(v)
}

// Sub returns v - other, element-wise. Panics if dimensions mismatch.
func (v FeatureVector) Sub(other FeatureVector) FeatureVector {
	if len(v) != len(other) {
		panic("recommender: feature vector dimension mismatch")
	}
	out := make(FeatureVector, len(v))
	for i := range v {
		out[i] = v[i] - other[i]
	}
	return out
}

// Scale returns v scaled by a scalar.
func (v FeatureVector) Scale(s float64) FeatureVector {
	out := make(FeatureVector, len(v))
	for i := range v {
		out[i] = v[i] * s
	}
	return out
}

// Add returns v + other, element-wise.
func (v FeatureVector) Add(other FeatureVector) FeatureVector {
	if len(v) != len(other) {
		panic("recommender: feature vector dimension mismatch")
	}
	out := make(FeatureVector, len(v))
	for i := range v {
		out[i] = v[i] + other[i]
	}
	return out
}

// Norm returns the L2 (Euclidean) norm of v.
func (v FeatureVector) Norm() float64 {
	var sum float64
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

// Dot returns the dot product of v and other.
func (v FeatureVector) Dot(other FeatureVector) float64 {
	if len(v) != len(other) {
		panic("recommender: feature vector dimension mismatch")
	}
	var sum float64
	for i := range v {
		sum += v[i] * other[i]
	}
	return sum
}

// Distance returns the Euclidean distance between v and other.
func (v FeatureVector) Distance(other FeatureVector) float64 {
	return v.Sub(other).Norm()
}

// SessionState tracks a listening session's current "vibe" as an EWMA
// centroid, a smoothed trajectory (recent direction of drift), and a
// spread estimate used to modulate exploration.
type SessionState struct {
	Centroid   FeatureVector
	Trajectory FeatureVector
	Spread     float64 // recency-weighted variance around the centroid

	initialized bool
}

// NewSessionState returns an empty session state for the given feature
// dimensionality. Call Update with the first track to seed it.
func NewSessionState(dim int) *SessionState {
	return &SessionState{
		Centroid:   make(FeatureVector, dim),
		Trajectory: make(FeatureVector, dim),
	}
}

// Update folds a newly played track's features into the session state
// using an exponentially weighted moving average. alpha controls how
// reactive the session is to new tracks; 0.2-0.3 is a reasonable default.
func (s *SessionState) Update(track FeatureVector, alpha float64) {
	if !s.initialized {
		// Seed directly from the first track — no history to blend with yet.
		s.Centroid = append(FeatureVector(nil), track...)
		s.Trajectory = make(FeatureVector, track.Dim())
		s.Spread = 0
		s.initialized = true
		return
	}

	prevCentroid := s.Centroid

	// centroid_t = alpha * track + (1 - alpha) * centroid_{t-1}
	s.Centroid = track.Scale(alpha).Add(prevCentroid.Scale(1 - alpha))

	// trajectory_t = alpha * (centroid_t - centroid_{t-1}) + (1 - alpha) * trajectory_{t-1}
	delta := s.Centroid.Sub(prevCentroid)
	s.Trajectory = delta.Scale(alpha).Add(s.Trajectory.Scale(1 - alpha))

	// spread_t = alpha * dist(track, centroid_t) + (1 - alpha) * spread_{t-1}
	dist := track.Distance(s.Centroid)
	s.Spread = alpha*dist + (1-alpha)*s.Spread
}

// ScoreWeights holds the weighted-sum coefficients for the three scoring
// terms. Defaults reflect continuity as the primary signal, trajectory as
// a secondary refinement, and novelty as mostly a tie-breaker/veto.
type ScoreWeights struct {
	Continuity float64
	Trajectory float64
	Novelty    float64
}

// DefaultWeights returns a reasonable starting point. Tune against real
// skip/complete/replay logs once you have them (e.g. logistic regression
// with "was it skipped" as the label and the three component scores as
// features).
func DefaultWeights() ScoreWeights {
	return ScoreWeights{Continuity: 0.5, Trajectory: 0.3, Novelty: 0.2}
}

// ScoreParams bundles the tunables for Score so callers aren't stuck with
// magic numbers scattered through call sites.
type ScoreParams struct {
	Weights ScoreWeights

	// TrajectoryGate: below this trajectory norm, momentum is treated as
	// noise rather than signal, and the trajectory term is zeroed out
	// instead of contributing a near-random direction.
	TrajectoryGate float64

	// Epsilon guards against divide-by-zero in the cosine similarity
	// denominator.
	Epsilon float64
}

// DefaultScoreParams returns sensible defaults for ScoreParams.
func DefaultScoreParams() ScoreParams {
	return ScoreParams{
		Weights:        DefaultWeights(),
		TrajectoryGate: 0.05,
		Epsilon:        1e-8,
	}
}

// ScoreBreakdown exposes the individual term values alongside the combined
// score, useful for debugging, logging, and later fitting weights against
// real feedback data.
type ScoreBreakdown struct {
	Continuity float64
	Trajectory float64
	Novelty    float64
	Combined   float64
}

// Continuity returns how close the candidate is to the session centroid,
// normalized to roughly [0, 1] regardless of feature dimensionality.
// Higher is better (closer).
func Continuity(candidate FeatureVector, state *SessionState) float64 {
	d := float64(candidate.Dim())
	if d == 0 {
		return 0
	}
	maxDist := math.Sqrt(d)
	dist := candidate.Distance(state.Centroid)
	return 1 - dist/maxDist
}

// TrajectoryAlignment returns the cosine similarity between the candidate's
// direction away from the centroid and the session's momentum vector.
// Returns 0 if the session has no established trajectory yet (gated by
// params.TrajectoryGate), rather than a noisy/random angle.
func TrajectoryAlignment(candidate FeatureVector, state *SessionState, params ScoreParams) float64 {
	tauNorm := state.Trajectory.Norm()
	if tauNorm <= params.TrajectoryGate {
		return 0
	}

	diff := candidate.Sub(state.Centroid)
	diffNorm := diff.Norm()
	denom := diffNorm*tauNorm + params.Epsilon
	if denom == 0 {
		return 0
	}
	return diff.Dot(state.Trajectory) / denom
}

// Novelty returns the normalized distance from the candidate to its
// nearest recently played track, clamped to [0, 1]. Higher means "more
// different from what we just played" — i.e. more novel.
func Novelty(candidate FeatureVector, recentlyPlayed []FeatureVector) float64 {
	d := float64(candidate.Dim())
	if d == 0 {
		return 0
	}
	maxDist := math.Sqrt(d)

	if len(recentlyPlayed) == 0 {
		// Nothing to compare against — treat as maximally novel.
		return 1
	}

	minDist := math.Inf(1)
	for _, r := range recentlyPlayed {
		dist := candidate.Distance(r)
		if dist < minDist {
			minDist = dist
		}
	}

	novelty := minDist / maxDist
	if novelty > 1 {
		novelty = 1
	}
	return novelty
}

// Score computes the full weighted combination of continuity, trajectory
// alignment, and novelty for a single candidate track.
func Score(candidate FeatureVector, state *SessionState, recentlyPlayed []FeatureVector, params ScoreParams) ScoreBreakdown {
	cont := Continuity(candidate, state)
	traj := TrajectoryAlignment(candidate, state, params)
	nov := Novelty(candidate, recentlyPlayed)

	combined := params.Weights.Continuity*cont +
		params.Weights.Trajectory*traj +
		params.Weights.Novelty*nov

	return ScoreBreakdown{
		Continuity: cont,
		Trajectory: traj,
		Novelty:    nov,
		Combined:   combined,
	}
}

// RankCandidates scores every candidate and returns them sorted by
// combined score, descending. Ties are broken by input order (stable sort).
func RankCandidates(candidates []FeatureVector, state *SessionState, recentlyPlayed []FeatureVector, params ScoreParams) []ScoredCandidate {
	scored := make([]ScoredCandidate, len(candidates))
	for i, c := range candidates {
		scored[i] = ScoredCandidate{
			Track: c,
			Score: Score(c, state, recentlyPlayed, params),
		}
	}
	sortByScoreDesc(scored)
	return scored
}

// ScoredCandidate pairs a candidate's features with its score breakdown.
type ScoredCandidate struct {
	Track FeatureVector
	Score ScoreBreakdown
}

// sortByScoreDesc sorts in place by Score.Combined, descending, using a
// stable insertion sort — candidate pools in this domain are small
// (tens of tracks), so this avoids pulling in "sort" just for stability
// guarantees you'd otherwise have to re-derive with sort.Slice + index tiebreak.
func sortByScoreDesc(s []ScoredCandidate) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1].Score.Combined < s[j].Score.Combined {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}
