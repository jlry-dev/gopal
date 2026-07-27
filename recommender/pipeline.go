package recommender

import (
	"errors"
	"math"
	"math/rand"
)

// SourceResult is one candidate source's ranked output for a given seed
// track — e.g. a content-similarity nearest-neighbor list, or a
// tag/artist-graph "related tracks" list. Tracks should already be
// ordered best-first; rank position (not any source-specific score) is
// what gets fused, since different sources' raw scores aren't
// comparable.
type SourceResult struct {
	Name   string
	Tracks []Track
	Weight float64 // relative trust in this source, e.g. 1.0, 0.8, 0.5
}

// CandidateEvidence accumulates how strongly each source vouches for a
// track, keyed by source name so it's inspectable/loggable.
type CandidateEvidence struct {
	Track         Track
	SourceSignals map[string]float64
}

// AgreementScore sums a track's fused signal across sources, capped so
// that appearing in every source can't let one track structurally
// dominate the pool. cap is typically ~1.5x a single source's max
// possible contribution.
func (ev *CandidateEvidence) AgreementScore(cap float64) float64 {
	var sum float64
	for _, v := range ev.SourceSignals {
		sum += v
	}
	if sum > cap {
		return cap
	}
	return sum
}

// FuseSources merges multiple ranked candidate lists into one evidence
// map, using reciprocal-rank fusion: a track's signal from a given
// source is weight * 1/(rank+1). This normalizes across sources with
// incompatible native scales (cosine similarity vs. a graph-walk score,
// for instance) since rank position is always comparable.
func FuseSources(results []SourceResult) map[string]*CandidateEvidence {
	candidates := make(map[string]*CandidateEvidence)
	for _, r := range results {
		for rank, t := range r.Tracks {
			ev, ok := candidates[t.ID]
			if !ok {
				ev = &CandidateEvidence{Track: t, SourceSignals: make(map[string]float64)}
				candidates[t.ID] = ev
			}
			ev.SourceSignals[r.Name] = r.Weight * (1.0 / float64(rank+1))
		}
	}
	return candidates
}

// PoolConfig controls how the fused candidate pool is trimmed before
// scoring.
type PoolConfig struct {
	MaxPerArtist   int     // hard cap on tracks from the same artist
	TargetSize     int     // desired pool size before discovery slots
	AgreementCap   float64 // cap passed to CandidateEvidence.AgreementScore
	DiscoverySlots int     // reserved slots for pure-discovery tracks
}

// DefaultPoolConfig returns reasonable starting values.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxPerArtist:   2,
		TargetSize:     35,
		AgreementCap:   1.5,
		DiscoverySlots: 5,
	}
}

// BuildPool ranks fused candidates by agreement score, excludes anything
// in excludeIDs (typically the recently-played window), and enforces a
// per-artist cap so one well-connected artist can't flood the pool.
func BuildPool(candidates map[string]*CandidateEvidence, cfg PoolConfig, excludeIDs map[string]bool) []Track {
	type scored struct {
		ev    *CandidateEvidence
		score float64
	}

	items := make([]scored, 0, len(candidates))
	for _, ev := range candidates {
		if excludeIDs[ev.Track.ID] {
			continue
		}
		items = append(items, scored{ev: ev, score: ev.AgreementScore(cfg.AgreementCap)})
	}

	// Stable descending sort by agreement score (small pools, insertion sort is fine).
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j-1].score < items[j].score {
			items[j-1], items[j] = items[j], items[j-1]
			j--
		}
	}

	pool := make([]Track, 0, cfg.TargetSize)
	artistCounts := make(map[string]int)
	for _, it := range items {
		if len(pool) >= cfg.TargetSize {
			break
		}
		if artistCounts[it.ev.Track.TrackArtist] >= cfg.MaxPerArtist {
			continue
		}
		pool = append(pool, it.ev.Track)
		artistCounts[it.ev.Track.TrackArtist]++
	}
	return pool
}

// AddDiscoverySlots appends a random sample from a pure-discovery pool.
// These are reserved slots rather than candidates competing on agreement
// score — discovery tracks structurally lose that competition (they
// appear in only one "source"), so without reservation they'd never
// survive BuildPool and the discovery source would be pointless.
func AddDiscoverySlots(pool []Track, discovery []Track, n int, rng *rand.Rand) []Track {
	if n > len(discovery) {
		n = len(discovery)
	}
	perm := rng.Perm(len(discovery))
	for i := 0; i < n; i++ {
		pool = append(pool, discovery[perm[i]])
	}
	return pool
}

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

// EventType identifies a playback outcome.
type EventType string

const (
	EventSkipEarly EventType = "skip_early" // skipped within the first ~5s
	EventSkipLate  EventType = "skip_late"
	EventCompleted EventType = "completed"
	EventReplayed  EventType = "replayed"
)

// RewardFor maps a playback event to a reward value. Treat these as a
// starting point — refit against real logs once you have them.
func RewardFor(event EventType) float64 {
	switch event {
	case EventSkipEarly:
		return -1.0
	case EventSkipLate:
		return -0.3
	case EventCompleted:
		return 1.0
	case EventReplayed:
		return 1.5
	default:
		return 0.0
	}
}

// RewardLogger is the persistence seam for feedback — implement this
// against SQLite, Postgres, whatever you're using, and the pipeline
// doesn't need to know about storage.
type RewardLogger interface {
	LogReward(trackID string, reward float64) error
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

// Pipeline holds everything needed to go from "a track just finished" to
// "here's the next track", plus feedback logging.
type Pipeline struct {
	State    *SessionState
	Recent   *RecentWindow
	Selector *Selector
	Params   ScoreParams
	Pool     PoolConfig
	Logger   RewardLogger // optional; nil is fine, feedback just won't persist
	alpha    float64
}

// NewPipeline wires up a fresh pipeline for a session with the given
// feature dimensionality. alpha is the EWMA reactivity for session state
// updates (0.2-0.3 is a reasonable default).
func NewPipeline(dim int, alpha float64, rng *rand.Rand, logger RewardLogger) *Pipeline {
	return &Pipeline{
		State:    NewSessionState(dim),
		Recent:   NewRecentWindow(15),
		Selector: DefaultSelector(rng),
		Params:   DefaultScoreParams(),
		Pool:     DefaultPoolConfig(),
		Logger:   logger,
		alpha:    alpha,
	}
}

// OnTrackPlayed folds a track into session state and the recent-played
// window. Call this whenever a track starts playing.
func (p *Pipeline) OnTrackPlayed(t Track) {
	p.State.Update(t.Features, p.alpha)
	p.Recent.Add(t)
}

// NextTrack runs the full candidate generation -> fusion -> pooling ->
// scoring -> selection sequence and returns the chosen track.
// sources should already be fetched (e.g. content-similarity and
// graph-similarity results for the current/seed track) — this function
// only fuses and scores, it doesn't do network I/O.
func (p *Pipeline) NextTrack(sources []SourceResult, discovery []Track) (Track, error) {
	fused := FuseSources(sources)
	pool := BuildPool(fused, p.Pool, p.Recent.IDs())
	pool = AddDiscoverySlots(pool, discovery, p.Pool.DiscoverySlots, p.Selector.Rand)

	if len(pool) == 0 {
		return Track{}, errors.New("recommender: no candidates available (check sources and fallback pool)")
	}

	ranked := RankTracks(pool, p.State, p.Recent.Features(), p.Params)
	return p.Selector.Select(ranked, p.State)
}

// OnFeedback records a playback outcome. Call this on skip/complete/replay.
func (p *Pipeline) OnFeedback(t Track, event EventType) error {
	if p.Logger == nil {
		return nil
	}
	return p.Logger.LogReward(t.ID, RewardFor(event))
}
