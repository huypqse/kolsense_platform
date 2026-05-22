package ingestion

import (
	"fmt"
	"os"

	"github.com/gen2brain/go-fitz"
)

// PDFParser extracts plain text from PDF files using go-fitz (mupdf).
type PDFParser struct{}

// NewPDFParser creates a new PDFParser.
func NewPDFParser() *PDFParser { return &PDFParser{} }

// PageText holds the extracted text for a single PDF page.
type PageText struct {
	PageNum int
	Text    string
}

// ParseFile extracts text page-by-page from a PDF file.
func (p *PDFParser) ParseFile(filePath string) ([]PageText, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("pdf parser: file not found: %s", filePath)
	}

	doc, err := fitz.New(filePath)
	if err != nil {
		return nil, fmt.Errorf("pdf parser: open pdf: %w", err)
	}
	defer doc.Close()

	var pages []PageText
	totalPage := doc.NumPage()

	for pageIndex := 0; pageIndex < totalPage; pageIndex++ {
		text, err := doc.Text(pageIndex)
		if err != nil {
			continue
		}

		if len(text) > 0 {
			pages = append(pages, PageText{
				PageNum: pageIndex + 1, // 1-indexed for the rest of the app
				Text:    text,
			})
		}
	}

	return pages, nil
}
