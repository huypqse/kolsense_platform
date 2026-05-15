package ingestion

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDFParser extracts plain text from PDF files using pdfcpu.
type PDFParser struct{}

// NewPDFParser creates a new PDFParser.
func NewPDFParser() *PDFParser { return &PDFParser{} }

// PageText holds the extracted text for a single PDF page.
type PageText struct {
	PageNum int
	Text    string
}

// ParseFile extracts text page-by-page from a PDF file.
// It returns one PageText per page that contains non-empty text.
func (p *PDFParser) ParseFile(filePath string) ([]PageText, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("pdf parser: file not found: %s", filePath)
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	// pdfcpu writes per-page text to an output directory.
	// We use a temp dir and read the results back.
	tmpDir, err := os.MkdirTemp("", "kolsense-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("pdf parser: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := api.ExtractContentFile(filePath, tmpDir, nil, conf); err != nil {
		// Fallback: try plain text extraction
		return p.fallbackExtract(filePath, conf)
	}

	return p.readExtractedPages(tmpDir)
}

// readExtractedPages reads text files written by pdfcpu into tmpDir.
func (p *PDFParser) readExtractedPages(dir string) ([]PageText, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("pdf parser: read dir: %w", err)
	}

	var pages []PageText
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		pageNum := parsePageNum(entry.Name())
		pages = append(pages, PageText{PageNum: pageNum, Text: text})
	}
	return pages, nil
}

// fallbackExtract uses pdfcpu's plain content stream extraction.
func (p *PDFParser) fallbackExtract(filePath string, conf *model.Configuration) ([]PageText, error) {
	tmpDir, err := os.MkdirTemp("", "kolsense-pdf-fb-*")
	if err != nil {
		return nil, fmt.Errorf("pdf parser fallback: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := api.ExtractPagesFile(filePath, tmpDir, nil, conf); err != nil {
		return nil, fmt.Errorf("pdf parser fallback: extract pages: %w", err)
	}
	return p.readExtractedPages(tmpDir)
}

// parsePageNum attempts to extract a page number from a filename like "page_003.txt".
func parsePageNum(name string) int {
	name = strings.TrimSuffix(name, ".txt")
	parts := strings.Split(name, "_")
	for i := len(parts) - 1; i >= 0; i-- {
		var n int
		if _, err := fmt.Sscanf(parts[i], "%d", &n); err == nil {
			return n
		}
	}
	return 0
}
