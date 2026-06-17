package recommender

import (
	"math"
	"sort"
)

type Vector struct {
	Valence float64
	Energy  float64
}

type ScoredTrack struct {
	ID       string
	Features Vector
	Distance float64
	Score    float64
}

// euclidean computes the L2 norm (Euclidean distance) between two mood vectors.
func euclidean(a, b Vector) float64 {
	dv := a.Valence - b.Valence
	de := a.Energy - b.Energy
	return math.Sqrt(dv*dv + de*de)
}

// Rank scores each candidate against the session vector by Euclidean distance
// and returns them sorted ascending (closest mood first).
func Rank(sessionRBID string, candidates []AudioFeatures) []ScoredTrack {
	sTrack := []ScoredTrack{}
	sessionTrack := ScoredTrack{
		ID: sessionRBID,
	}

	// separate the session track with the candidates
	for _, t := range candidates {
		v := Vector{
			Valence: t.Valence,
			Energy:  t.Energy,
		}

		if sessionRBID == t.ID {
			sessionTrack.Features = v
			continue
		}

		st := ScoredTrack{
			ID:       t.ID,
			Features: v,
		}

		sTrack = append(sTrack, st)

	}

	for i := range sTrack {
		sTrack[i].Distance = euclidean(sessionTrack.Features, sTrack[i].Features)
	}
	sort.Slice(sTrack, func(i, j int) bool {
		return sTrack[i].Distance < sTrack[j].Distance
	})

	return sTrack
}
