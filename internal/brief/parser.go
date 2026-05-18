package brief

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"kolsense/internal/llm"
)

// Parser extracts structured fields from free-text campaign briefs.
// Strategy: rule-based extraction first; if confidence < threshold, fall back to LLM.
type Parser struct {
	llmClient llm.Client
	threshold float64 // confidence threshold for LLM fallback (0.0–1.0)
}

// NewParser creates a hybrid Parser.
func NewParser(llmClient llm.Client, confidenceThreshold float64) *Parser {
	return &Parser{
		llmClient: llmClient,
		threshold: confidenceThreshold,
	}
}

// Parse extracts a CampaignBrief from raw text.
// It uses rule-based extraction first, and falls back to the LLM if confidence is low.
func (p *Parser) Parse(ctx context.Context, rawText string) (*CampaignBrief, error) {
	brief, confidence := ruleBasedParse(rawText)
	brief.RawText = rawText
	brief.Confidence = confidence

	if confidence >= p.threshold {
		brief.ParseStrategy = "rule_based"
		return brief, nil
	}

	// LLM fallback
	llmBrief, err := p.llmFallback(ctx, rawText)
	if err != nil {
		// If LLM fails, return whatever rule-based extracted
		brief.ParseStrategy = "rule_based_degraded"
		return brief, nil
	}
	llmBrief.RawText = rawText
	llmBrief.ParseStrategy = "llm_fallback"
	llmBrief.Confidence = confidence
	return llmBrief, nil
}

// ─── Rule-based extraction ──────────────────────────────────────────────────────

var (
	// Platforms
	reTikTok    = regexp.MustCompile(`(?i)\btiktok\b`)
	reInstagram = regexp.MustCompile(`(?i)\binstagram\b`)
	reYouTube   = regexp.MustCompile(`(?i)\byoutube\b`)
	reFacebook  = regexp.MustCompile(`(?i)\bfacebook\b`)

	// Categories
	reBeauty    = regexp.MustCompile(`(?i)\b(beauty|làm đẹp|mỹ phẩm|skincare|makeup)\b`)
	reFashion   = regexp.MustCompile(`(?i)\b(fashion|thời trang|outfit)\b`)
	reLifestyle = regexp.MustCompile(`(?i)\b(lifestyle|phong cách sống)\b`)
	reTech      = regexp.MustCompile(`(?i)\b(tech|công nghệ|technology)\b`)
	reFood      = regexp.MustCompile(`(?i)\b(food|ẩm thực|đồ ăn)\b`)
	reFitness   = regexp.MustCompile(`(?i)\b(fitness|gym|sức khỏe|sport)\b`)
	reTravel    = regexp.MustCompile(`(?i)\b(travel|du lịch)\b`)

	// Goals
	reAwareness  = regexp.MustCompile(`(?i)\b(awareness|nhận diện thương hiệu|brand awareness)\b`)
	reConversion = regexp.MustCompile(`(?i)\b(conversion|chuyển đổi|bán hàng|doanh thu)\b`)
	reEngagement = regexp.MustCompile(`(?i)\b(engagement|tương tác)\b`)
	reBranding   = regexp.MustCompile(`(?i)\b(branding|thương hiệu)\b`)

	// Budget: captures numbers followed by VND/triệu/tỷ keywords
	reBudget       = regexp.MustCompile(`(?i)ngân sách[:\s]*(\d[\d.,]*)\s*(triệu|tỷ|million|billion|vnd|đồng)?[\s\-–]+(\d[\d.,]*)\s*(triệu|tỷ|million|billion|vnd|đồng)?`)
	reBudgetSingle = regexp.MustCompile(`(?i)ngân sách[:\s]*(\d[\d.,]*)\s*(triệu|tỷ|million|billion|vnd|đồng)?`)

	// Gender
	reFemale = regexp.MustCompile(`(?i)\b(nữ|female|women|phụ nữ)\b`)
	reMale   = regexp.MustCompile(`(?i)\b(nam|male|men)\b`)

	// Age range
	reAge = regexp.MustCompile(`(?i)(?:tuổi|age)[:\s]*(\d{2})\s*[-–]\s*(\d{2})`)
)

func ruleBasedParse(text string) (*CampaignBrief, float64) {
	brief := &CampaignBrief{}
	signals := 0
	found := 0

	// Platforms
	signals++
	platforms := extractPlatforms(text)
	if len(platforms) > 0 {
		brief.Platforms = platforms
		found++
	}

	// Categories
	signals++
	cats := extractCategories(text)
	if len(cats) > 0 {
		brief.Categories = cats
		found++
	}

	// Goals
	signals++
	if goal := extractGoal(text); goal != "" {
		brief.Goal = goal
		found++
	}

	// Budget
	signals++
	if min, max := extractBudget(text); max > 0 {
		brief.Budget = BudgetRange{MinVND: min, MaxVND: max}
		found++
	}

	// Gender
	signals++
	if gender := extractGender(text); gender != "" {
		brief.Audience.Gender = gender
		found++
	}

	// Age
	signals++
	if lo, hi := extractAge(text); lo > 0 && hi > 0 {
		brief.Audience.AgeRange = [2]int{lo, hi}
		found++
	}

	confidence := float64(found) / float64(signals)
	return brief, confidence
}

func extractPlatforms(text string) []Platform {
	var p []Platform
	if reTikTok.MatchString(text) {
		p = append(p, PlatformTikTok)
	}
	if reInstagram.MatchString(text) {
		p = append(p, PlatformInstagram)
	}
	if reYouTube.MatchString(text) {
		p = append(p, PlatformYouTube)
	}
	if reFacebook.MatchString(text) {
		p = append(p, PlatformFacebook)
	}
	return p
}

func extractCategories(text string) []Category {
	var c []Category
	if reBeauty.MatchString(text) {
		c = append(c, CategoryBeauty)
	}
	if reFashion.MatchString(text) {
		c = append(c, CategoryFashion)
	}
	if reLifestyle.MatchString(text) {
		c = append(c, CategoryLifestyle)
	}
	if reTech.MatchString(text) {
		c = append(c, CategoryTech)
	}
	if reFood.MatchString(text) {
		c = append(c, CategoryFood)
	}
	if reFitness.MatchString(text) {
		c = append(c, CategoryFitness)
	}
	if reTravel.MatchString(text) {
		c = append(c, CategoryTravel)
	}
	return c
}

func extractGoal(text string) CampaignGoal {
	switch {
	case reConversion.MatchString(text):
		return GoalConversion
	case reAwareness.MatchString(text):
		return GoalAwareness
	case reEngagement.MatchString(text):
		return GoalEngagement
	case reBranding.MatchString(text):
		return GoalBranding
	}
	return ""
}

func extractGender(text string) string {
	hasFemale := reFemale.MatchString(text)
	hasMale := reMale.MatchString(text)
	switch {
	case hasFemale && hasMale:
		return "all"
	case hasFemale:
		return "female"
	case hasMale:
		return "male"
	}
	return ""
}

func extractAge(text string) (int, int) {
	m := reAge.FindStringSubmatch(text)
	if len(m) < 3 {
		return 0, 0
	}
	lo, _ := strconv.Atoi(m[1])
	hi, _ := strconv.Atoi(m[2])
	return lo, hi
}

func extractBudget(text string) (int64, int64) {
	// Try range first
	if m := reBudget.FindStringSubmatch(text); len(m) >= 4 {
		min := parseVND(m[1], m[2])
		max := parseVND(m[3], m[4])
		if max > 0 {
			return min, max
		}
	}
	// Single value
	if m := reBudgetSingle.FindStringSubmatch(text); len(m) >= 3 {
		v := parseVND(m[1], m[2])
		return 0, v
	}
	return 0, 0
}

func parseVND(numStr, unit string) int64 {
	numStr = strings.ReplaceAll(numStr, ",", "")
	numStr = strings.ReplaceAll(numStr, ".", "")
	v, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(unit) {
	case "triệu", "million":
		return v * 1_000_000
	case "tỷ", "billion":
		return v * 1_000_000_000
	default:
		return v
	}
}

// ─── LLM fallback ───────────────────────────────────────────────────────────────

const llmExtractionSystemPrompt = `Bạn là trợ lý phân tích brief marketing. 
Hãy trích xuất thông tin từ brief và trả về JSON với cấu trúc sau:
{
  "categories": ["beauty","fashion","lifestyle","tech","food","fitness","travel"],
  "platforms":  ["tiktok","instagram","youtube","facebook"],
  "goal":       "awareness|conversion|engagement|branding",
  "budget_min_vnd": <number>,
  "budget_max_vnd": <number>,
  "gender":     "female|male|all",
  "age_min":    <number>,
  "age_max":    <number>,
  "regions":    ["HN","HCM","nationwide"]
}
Chỉ trả về JSON, không giải thích thêm.`

type llmExtractionResult struct {
	Categories   []string `json:"categories"`
	Platforms    []string `json:"platforms"`
	Goal         string   `json:"goal"`
	BudgetMinVND int64    `json:"budget_min_vnd"`
	BudgetMaxVND int64    `json:"budget_max_vnd"`
	Gender       string   `json:"gender"`
	AgeMin       int      `json:"age_min"`
	AgeMax       int      `json:"age_max"`
	Regions      []string `json:"regions"`
}

func (p *Parser) llmFallback(ctx context.Context, rawText string) (*CampaignBrief, error) {
	resp, err := p.llmClient.Complete(ctx, llm.CompletionRequest{
		SystemPrompt: llmExtractionSystemPrompt,
		UserPrompt:   fmt.Sprintf("Brief:\n%s", rawText),
		MaxTokens:    512,
		Temperature:  0.1,
	})
	if err != nil {
		return nil, fmt.Errorf("llm fallback: %w", err)
	}

	// Extract JSON from response (model may wrap it in markdown)
	jsonStr := extractJSON(resp.Text)
	var result llmExtractionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("llm fallback: parse JSON: %w", err)
	}

	brief := &CampaignBrief{
		Budget: BudgetRange{MinVND: result.BudgetMinVND, MaxVND: result.BudgetMaxVND},
		Goal:   CampaignGoal(result.Goal),
		Audience: AudienceSpec{
			Gender:  result.Gender,
			Regions: result.Regions,
		},
	}
	if result.AgeMin > 0 || result.AgeMax > 0 {
		brief.Audience.AgeRange = [2]int{result.AgeMin, result.AgeMax}
	}
	for _, c := range result.Categories {
		brief.Categories = append(brief.Categories, Category(c))
	}
	for _, pl := range result.Platforms {
		brief.Platforms = append(brief.Platforms, Platform(pl))
	}
	return brief, nil
}

// extractJSON pulls the first JSON object from a string (handles markdown code blocks).
func extractJSON(s string) string {
	// Strip markdown fences
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "```json"); idx >= 0 {
		s = s[idx+7:]
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	} else if idx := strings.Index(s, "```"); idx >= 0 {
		s = s[idx+3:]
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	// Find first { ... }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end <= start {
		return s
	}
	return s[start : end+1]
}
