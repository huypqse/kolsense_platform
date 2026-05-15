package scoring

import (
	"math"
	"sort"

	"kolsense/internal/brief"
	"kolsense/internal/vectorstore"
)

// Scorer computes composite fit scores for KOL candidates.
type Scorer struct {
	weights Weights
}

// NewScorer creates a Scorer with the given weight configuration.
func NewScorer(w Weights) *Scorer { return &Scorer{weights: w} }

// ScoreInput bundles all the data needed to score a set of KOL candidates.
type ScoreInput struct {
	Brief    *brief.CampaignBrief
	Profiles map[string]vectorstore.KOLProfile // keyed by KOL name
	SimScores map[string]float64               // keyed by KOL name
	TopChunks map[string][]string              // keyed by KOL name
}

// Score computes a ScoredKOL for every profile in the input and returns them
// sorted by descending FitScore with 1-based Rank assigned.
func (s *Scorer) Score(in ScoreInput) []ScoredKOL {
	// Normalisation ranges derived from the candidate set
	maxEngagement := maxEngagementRate(in.Profiles)
	maxROI        := maxROI(in.Profiles)

	var results []ScoredKOL
	for name, profile := range in.Profiles {
		simScore := in.SimScores[name]
		bd := ScoreBreakdown{
			Similarity:    simScore,
			Engagement:    normalise(profile.AvgEngagement, 0, maxEngagement),
			ROI:           normalise(profile.AvgROI, 0, maxROI),
			BudgetFit:     budgetFit(profile, in.Brief.Budget),
			AudienceMatch: 0.5, // placeholder — extend with demographic data
		}
		fit := s.weights.Similarity*bd.Similarity +
			s.weights.Engagement*bd.Engagement +
			s.weights.ROI*bd.ROI +
			s.weights.BudgetFit*bd.BudgetFit +
			s.weights.AudienceMatch*bd.AudienceMatch

		results = append(results, ScoredKOL{
			Profile:   profile,
			SimScore:  simScore,
			FitScore:  math.Round(fit*1000) / 1000,
			Breakdown: bd,
			TopChunks: in.TopChunks[name],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FitScore > results[j].FitScore
	})
	for i := range results {
		results[i].Rank = i + 1
	}
	return results
}

// normalise maps v into [0,1] given a [min,max] range.
func normalise(v, min, max float64) float64 {
	if max <= min {
		return 0
	}
	n := (v - min) / (max - min)
	if n < 0 { return 0 }
	if n > 1 { return 1 }
	return n
}

// budgetFit returns 1.0 if the KOL fee is within budget, decays linearly outside.
func budgetFit(p vectorstore.KOLProfile, b brief.BudgetRange) float64 {
	if p.FeeMinVND == 0 && p.FeeMaxVND == 0 {
		return 0.5 // unknown fee
	}
	if b.MaxVND == 0 {
		return 0.5 // no budget constraint
	}
	// Full fit: KOL fee max <= brief budget max
	if p.FeeMaxVND <= b.MaxVND {
		return 1.0
	}
	// Partial fit: fee slightly over budget — linear decay
	overRatio := float64(p.FeeMaxVND-b.MaxVND) / float64(b.MaxVND)
	fit := 1.0 - overRatio
	if fit < 0 { return 0 }
	return fit
}

func maxEngagementRate(profiles map[string]vectorstore.KOLProfile) float64 {
	var max float64
	for _, p := range profiles {
		if p.AvgEngagement > max { max = p.AvgEngagement }
	}
	return max
}

func maxROI(profiles map[string]vectorstore.KOLProfile) float64 {
	var max float64
	for _, p := range profiles {
		if p.AvgROI > max { max = p.AvgROI }
	}
	return max
}
