package main

import (
	"fmt"
	"strings"

	"github.com/dslipak/pdf"
)

func main() {
	r, err := pdf.Open("data/Glow_Forward_Campaign_Report.pdf.pdf")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	totalPage := r.NumPage()
	totalWords := 0
	for i := 1; i <= totalPage; i++ {
		p := r.Page(i)
		text, _ := p.GetPlainText(nil)
		words := len(strings.Fields(text))
		totalWords += words
		fmt.Printf("Page %d: %d words\n", i, words)
	}
	fmt.Printf("Total words: %d\n", totalWords)
}
