// Package prompt assembles RAG context prompts for the Gemini LLM.
package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"kolsense/internal/brief"
	"kolsense/internal/scoring"
)

// KOLReportData is the template data for the KOL recommendation report.
type KOLReportData struct {
	Brief     *brief.CampaignBrief
	Shortlist []scoring.ScoredKOL
	TopN      int
}

const systemPrompt = `Bạn là chuyên gia marketing KOL với hơn 10 năm kinh nghiệm tại thị trường Việt Nam.
Nhiệm vụ của bạn là phân tích brief chiến dịch và đề xuất KOL phù hợp nhất.
Hãy trả lời bằng tiếng Việt, súc tích, chuyên nghiệp và có căn cứ từ dữ liệu.`

var reportTemplate = template.Must(template.New("kol_report").Funcs(template.FuncMap{
	"pct":   func(f float64) string { return fmt.Sprintf("%.1f%%", f*100) },
	"score": func(f float64) string { return fmt.Sprintf("%.3f", f) },
	"vnd": func(v int64) string {
		if v == 0 {
			return "không xác định"
		}
		if v >= 1_000_000_000 {
			return fmt.Sprintf("%.1f tỷ", float64(v)/1_000_000_000)
		}
		return fmt.Sprintf("%.0f triệu", float64(v)/1_000_000)
	},
}).Parse(`=== CAMPAIGN BRIEF ===
Danh mục: {{range .Brief.Categories}}{{.}} {{end}}
Nền tảng: {{range .Brief.Platforms}}{{.}} {{end}}
Mục tiêu: {{.Brief.Goal}}
Ngân sách: {{vnd .Brief.Budget.MinVND}} – {{vnd .Brief.Budget.MaxVND}}
Đối tượng: {{.Brief.Audience.Gender}}, tuổi {{index .Brief.Audience.AgeRange 0}}–{{index .Brief.Audience.AgeRange 1}}

=== TOP {{.TopN}} KOL ỨNG VIÊN ===
{{range .Shortlist}}
[{{.Rank}}] {{.Profile.Name}} — Fit Score: {{score .FitScore}}
  Platform: {{range .Profile.Platform}}{{.}} {{end}}
  Followers: {{.Profile.FollowerCount}} | Engagement: {{pct .Profile.AvgEngagement}} | ROI: {{score .Profile.AvgROI}}x
  Phí: {{vnd .Profile.FeeMinVND}} – {{vnd .Profile.FeeMaxVND}}
  Score breakdown: sim={{score .Breakdown.Similarity}} eng={{score .Breakdown.Engagement}} roi={{score .Breakdown.ROI}} budget={{score .Breakdown.BudgetFit}}
  Nội dung liên quan:
{{range .TopChunks}}  • {{.}}
{{end}}
{{end}}

=== YÊU CẦU ===
Hãy viết báo cáo đề xuất KOL bao gồm:
1. Shortlist Top {{.TopN}} KOL với lý do cụ thể cho từng người
2. Điểm mạnh và điểm yếu của mỗi KOL
3. Khuyến nghị phân bổ ngân sách giữa các KOL
4. Rủi ro cần lưu ý và cách giảm thiểu
5. Kết luận và đề xuất ưu tiên`))

// Builder assembles the system prompt and user prompt for the LLM.
type Builder struct{}

// NewBuilder creates a new prompt Builder.
func NewBuilder() *Builder { return &Builder{} }

// Build returns the system prompt and user prompt for the given report data.
func (b *Builder) Build(data KOLReportData) (system, user string, err error) {
	if data.TopN == 0 {
		data.TopN = len(data.Shortlist)
	}
	// Trim shortlist to TopN
	if len(data.Shortlist) > data.TopN {
		data.Shortlist = data.Shortlist[:data.TopN]
	}

	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("prompt builder: render template: %w", err)
	}
	return strings.TrimSpace(systemPrompt), buf.String(), nil
}
