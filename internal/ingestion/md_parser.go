package ingestion

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"kolsense/internal/vectorstore"
)

// MDParser extracts plain text sections from Markdown files.
// It splits on horizontal-rule separators (---) so each document section
// becomes one "page", preserving the paragraph-aware chunker's ability
// to operate within meaningful semantic boundaries.
type MDParser struct{}

// NewMDParser creates a new MDParser.
func NewMDParser() *MDParser { return &MDParser{} }

// ParseFile reads a Markdown file and returns one PageText per section.
// Sections are delimited by lines that consist solely of "---".
func (p *MDParser) ParseFile(filePath string) ([]PageText, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("md parser: read file %s: %w", filePath, err)
	}

	sections := splitMarkdownSections(string(data))

	var pages []PageText
	pageNum := 1
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		pages = append(pages, PageText{
			PageNum: pageNum,
			Text:    section,
		})
		pageNum++
	}

	if len(pages) == 0 {
		return nil, fmt.Errorf("md parser: no content found in %s", filePath)
	}
	return pages, nil
}

// splitMarkdownSections splits Markdown content on bare "---" horizontal rules.
// Lines that are purely "---" (possibly with surrounding whitespace) act as
// section boundaries and are not included in the output.
func splitMarkdownSections(content string) []string {
	lines := strings.Split(content, "\n")

	var sections []string
	var cur strings.Builder

	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if cur.Len() > 0 {
				sections = append(sections, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteString(line)
			cur.WriteString("\n")
		}
	}
	if cur.Len() > 0 {
		sections = append(sections, cur.String())
	}
	return sections
}

// ── Profile-card extraction ────────────────────────────────────────────────────

// ExtractProfileFromMD reads a KOL profile Markdown file and builds a
// vectorstore.KOLProfile by parsing the "## Summary Profile Card" table.
// Returns nil when no card is found (e.g. for brand-wide documents).
func ExtractProfileFromMD(filePath, kolName string) *vectorstore.KOLProfile {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}

	content := string(data)

	const cardHeader = "## Summary Profile Card"
	idx := strings.Index(content, cardHeader)
	if idx < 0 {
		return nil
	}

	section := content[idx+len(cardHeader):]
	// Trim to the next top-level section header so we don't bleed into other content.
	if end := strings.Index(section, "\n## "); end > 0 {
		section = section[:end]
	}

	profile := &vectorstore.KOLProfile{
		Name:     kolName,
		Platform: []string{},
		Category: []string{},
		Metadata: map[string]any{},
	}

	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		key := strings.TrimSpace(parts[1])
		val := strings.TrimSpace(parts[2])

		if key == "" || key == "Thuộc tính" || key == "---" || strings.Contains(key, "---") {
			continue // skip header / separator rows
		}

		keyLow := strings.ToLower(key)
		switch {
		case keyLow == "platforms":
			for _, p := range strings.Split(val, ",") {
				p = strings.ToLower(strings.TrimSpace(p))
				if p != "" {
					profile.Platform = append(profile.Platform, p)
				}
			}

		case keyLow == "categories":
			for _, c := range strings.Split(val, ",") {
				c = strings.ToLower(strings.TrimSpace(c))
				if c != "" {
					profile.Category = append(profile.Category, c)
				}
			}

		case keyLow == "total followers":
			profile.FollowerCount = parseVNNumber(val)

		case strings.HasPrefix(keyLow, "blended engagement"):
			// "5.6% (0.056)" → extract decimal in parentheses first, fall back to pct
			profile.AvgEngagement = parseEngagement(val)

		case strings.HasPrefix(keyLow, "average roi"):
			// "5.8x" → 5.8
			profile.AvgROI = parseROI(val)

		case keyLow == "fee min (vnd)":
			profile.FeeMinVND = parseVNNumber(val)

		case keyLow == "fee max (vnd)":
			profile.FeeMaxVND = parseVNNumber(val)
		}
	}

	return profile
}

// ── Number-parsing helpers ─────────────────────────────────────────────────────

// reParenDecimal matches the decimal value inside parentheses, e.g. (0.056).
var reParenDecimal = regexp.MustCompile(`\(([0-9]+\.[0-9]+)\)`)

// parseEngagement handles formats:
//
//	"5.6% (0.056)"  → 0.056   (preferred — already a ratio)
//	"5.6%"          → 0.056   (convert pct to ratio)
//	"0.056"         → 0.056
func parseEngagement(s string) float64 {
	// Prefer the parenthesised decimal ratio when present.
	if m := reParenDecimal.FindStringSubmatch(s); len(m) == 2 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			return v
		}
	}
	// Strip % and convert percentage to ratio.
	s = strings.TrimSpace(strings.ReplaceAll(s, "%", ""))
	s = strings.Split(s, " ")[0] // take first token
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if v > 1 { // it's still a percentage like 5.6
		return v / 100
	}
	return v
}

// parseROI handles "5.8x" → 5.8, "4.6" → 4.6.
func parseROI(s string) float64 {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, "x")
	s = strings.Split(s, " ")[0]
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// parseVNNumber handles Vietnamese number formatting where "." is the
// thousands separator: "225.000.000" → 225000000.
// Also handles plain integers and numbers with commas.
func parseVNNumber(s string) int64 {
	// Remove everything after a space (e.g. "1.400.000 (TikTok + Instagram)")
	s = strings.Split(strings.TrimSpace(s), " ")[0]
	// Remove leading ~ or other non-numeric prefixes
	s = strings.TrimLeft(s, "~")
	// Determine separator style:
	// If the string contains both . and , use , as decimal (not our case here).
	// Vietnamese format: dots are thousands separators, no decimal.
	// Remove dots (thousands separator).
	s = strings.ReplaceAll(s, ".", "")
	// Remove commas as well (alternative thousands separator in some locales).
	s = strings.ReplaceAll(s, ",", "")
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
