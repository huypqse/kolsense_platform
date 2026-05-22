package scoring

import "kolsense/internal/vectorstore"

// Weights defines the contribution of each signal to the composite fit score.
type Weights struct {
	Similarity    float64
	Engagement    float64
	ROI           float64
	BudgetFit     float64
	AudienceMatch float64
}

// DefaultWeights returns the production-tuned weight configuration.
func DefaultWeights() Weights {
	return Weights{
		Similarity:    0.40,
		Engagement:    0.25,
		ROI:           0.20,
		BudgetFit:     0.10,
		AudienceMatch: 0.05,
	}
}

// ScoreBreakdown contains the per-signal normalized score (0–1).
type ScoreBreakdown struct {
	Similarity    float64 `json:"similarity"`
	Engagement    float64 `json:"engagement"`
	ROI           float64 `json:"roi"`
	BudgetFit     float64 `json:"budget_fit"`
	AudienceMatch float64 `json:"audience_match"`
}

// ScoredKOL is a KOL profile annotated with similarity and composite fit scores.
type ScoredKOL struct {
	Profile   vectorstore.KOLProfile
	SimScore  float64
	FitScore  float64
	Rank      int
	Breakdown ScoreBreakdown
	TopChunks []string
}
