// Package brief defines the CampaignBrief domain model and its field types.
package brief

import (
	"fmt"
	"time"
)

// Platform represents a social media platform.
type Platform string

const (
	PlatformTikTok    Platform = "tiktok"
	PlatformInstagram Platform = "instagram"
	PlatformYouTube   Platform = "youtube"
	PlatformFacebook  Platform = "facebook"
)

// Category represents a KOL / campaign content category.
type Category string

const (
	CategoryBeauty    Category = "beauty"
	CategoryFashion   Category = "fashion"
	CategoryLifestyle Category = "lifestyle"
	CategoryTech      Category = "tech"
	CategoryFood      Category = "food"
	CategoryFitness   Category = "fitness"
	CategoryTravel    Category = "travel"
)

// CampaignGoal represents the primary objective of the campaign.
type CampaignGoal string

const (
	GoalAwareness  CampaignGoal = "awareness"
	GoalConversion CampaignGoal = "conversion"
	GoalEngagement CampaignGoal = "engagement"
	GoalBranding   CampaignGoal = "branding"
)

// BudgetRange represents a VND budget window.
type BudgetRange struct {
	MinVND int64
	MaxVND int64
}

// AudienceSpec describes the target audience of the campaign.
type AudienceSpec struct {
	AgeRange [2]int   // e.g. [18, 35]
	Gender   string   // female | male | all
	Regions  []string // HN | HCM | nationwide | ...
	Interests []string
}

// CampaignBrief is the structured output of the brief parser.
type CampaignBrief struct {
	// RawText is the original marketer input.
	RawText string

	Categories []Category
	Budget     BudgetRange
	Platforms  []Platform
	Audience   AudienceSpec
	Goal       CampaignGoal
	StartDate  *time.Time
	EndDate    *time.Time

	// ParseStrategy records whether rule_based or llm_fallback was used.
	ParseStrategy string
	// Confidence is the rule-based extraction confidence score (0–1).
	Confidence float64
}

// BriefSummary returns a concise Vietnamese text representation of the brief
// suitable for embedding into the same vector space as KOL content.
func (b *CampaignBrief) BriefSummary() string {
	cats := joinCategories(b.Categories)
	plats := joinPlatforms(b.Platforms)
	return "Chiến dịch " + cats + " trên " + plats +
		", mục tiêu " + string(b.Goal) +
		", đối tượng " + b.Audience.Gender +
		" tuổi " + formatAgeRange(b.Audience.AgeRange) +
		", khu vực " + joinStrings(b.Audience.Regions)
}

func joinCategories(cs []Category) string {
	ss := make([]string, len(cs))
	for i, c := range cs { ss[i] = string(c) }
	return joinStrings(ss)
}

func joinPlatforms(ps []Platform) string {
	ss := make([]string, len(ps))
	for i, p := range ps { ss[i] = string(p) }
	return joinStrings(ss)
}

func joinStrings(ss []string) string {
	if len(ss) == 0 { return "không xác định" }
	result := ss[0]
	for _, s := range ss[1:] { result += ", " + s }
	return result
}

func formatAgeRange(r [2]int) string {
	if r[0] == 0 && r[1] == 0 { return "không xác định" }
	return fmt.Sprintf("%d–%d", r[0], r[1])
}
