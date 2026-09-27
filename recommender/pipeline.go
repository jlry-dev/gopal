package recommender

import (
	"errors"
	"math"
	"math/rand"
)

// RankedTrack pairs a Track with its full score breakdown.
type RankedTrack struct {
	Track Track
	Score ScoreBreakdown
}

// RankTracks scores every track in the pool against session state and
// returns them sorted by combined score, descending.
func RankTracks(pool []Track, state *SessionState, recentlyPlayed []FeatureVector, params ScoreParams) []RankedTrack {
	ranked := make([]RankedTrack, len(pool))
	for i, t := range pool {
		ranked[i] = RankedTrack{Track: t, Score: Score(t.Features, state, recentlyPlayed, params)}
	}
	for i := 1; i < len(ranked); i++ {
		j := i
		for j > 0 && ranked[j-1].Score.Combined < ranked[j].Score.Combined {
			ranked[j-1], ranked[j] = ranked[j], ranked[j-1]
			j--
		}
	}
	return ranked
}

// Selector implements epsilon-greedy selection over ranked candidates,
// with epsilon decaying as the session's vibe (spread) tightens.
type Selector struct {
	BaseEpsilon float64
	LambdaDecay float64
	MinEpsilon  float64
	Rand        *rand.Rand
}

// DefaultSelector returns a Selector with reasonable starting parameters.
// Seed the rand source yourself if you need reproducibility in tests.
func DefaultSelector(rng *rand.Rand) *Selector {
	return &Selector{
		BaseEpsilon: 0.3,
		LambdaDecay: 1.0,
		MinEpsilon:  0.05,
		Rand:        rng,
	}
}

// Epsilon computes the current exploration rate. A tight session (low
// Spread) pushes epsilon down toward MinEpsilon; a loose/undecided
// session keeps epsilon near BaseEpsilon.
func (s *Selector) Epsilon(state *SessionState) float64 {
	spreadInverse := 1.0 / (state.Spread + 1e-6)
	eps := s.BaseEpsilon * math.Exp(-s.LambdaDecay*spreadInverse)
	if eps < s.MinEpsilon {
		eps = s.MinEpsilon
	}
	if eps > s.BaseEpsilon {
		eps = s.BaseEpsilon
	}
	return eps
}

// Select picks a track from the ranked pool: exploit (top score) with
// probability 1-epsilon, or explore (score-weighted random pick, not
// uniform — a bad candidate should still rarely win) with probability
// epsilon.
func (s *Selector) Select(ranked []RankedTrack, state *SessionState) (Track, error) {
	if len(ranked) == 0 {
		return Track{}, errors.New("recommender: cannot select from an empty pool")
	}

	if s.Rand.Float64() < s.Epsilon(state) {
		return s.weightedRandomChoice(ranked), nil
	}
	return ranked[0].Track, nil
}

func (s *Selector) weightedRandomChoice(ranked []RankedTrack) Track {
	// Shift scores positive (Score.Combined can be slightly negative)
	// so every candidate has a nonzero chance, weighted by quality.
	minScore := ranked[len(ranked)-1].Score.Combined
	shift := 0.0
	if minScore < 0 {
		shift = -minScore + 0.01
	}

	weights := make([]float64, len(ranked))
	var total float64
	for i, rt := range ranked {
		w := rt.Score.Combined + shift
		if w < 0 {
			w = 0
		}
		weights[i] = w
		total += w
	}

	if total <= 0 {
		return ranked[s.Rand.Intn(len(ranked))].Track
	}

	r := s.Rand.Float64() * total
	var cum float64
	for i, w := range weights {
		cum += w
		if r <= cum {
			return ranked[i].Track
		}
	}
	return ranked[len(ranked)-1].Track
}

// RecentWindow is a fixed-size ring of the last N played tracks, used
// both for novelty scoring and hard exclusion from future candidate pools.
type RecentWindow struct {
	size   int
	tracks []Track
}

// NewRecentWindow creates a window that retains up to size tracks.
func NewRecentWindow(size int) *RecentWindow {
	return &RecentWindow{size: size, tracks: make([]Track, 0, size)}
}

// Add records a newly played track, evicting the oldest if at capacity.
func (w *RecentWindow) Add(t Track) {
	w.tracks = append(w.tracks, t)
	if len(w.tracks) > w.size {
		w.tracks = w.tracks[len(w.tracks)-w.size:]
	}
}

// Features returns the feature vectors of everything currently in the
// window, for novelty scoring.
func (w *RecentWindow) Features() []FeatureVector {
	out := make([]FeatureVector, len(w.tracks))
	for i, t := range w.tracks {
		out[i] = t.Features
	}
	return out
}

// IDs returns the set of track IDs currently in the window, for hard
// exclusion from candidate pools.
func (w *RecentWindow) IDs() map[string]bool {
	ids := make(map[string]bool, len(w.tracks))
	for _, t := range w.tracks {
		ids[t.ID] = true
	}
	return ids
}
