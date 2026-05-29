package rag

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/lu4p/cat"
	"github.com/xuri/excelize/v2"
)

// ExtractText extracts plain text from a file based on its extension.
// Supported: .pdf, .docx, .xlsx
func ExtractText(path, ext string) (string, error) {
	switch ext {
	case ".pdf":
		return extractPDF(path)
	case ".docx":
		return extractDOCX(path)
	case ".xlsx":
		return extractExcel(path)
	default:
		return "", fmt.Errorf("unsupported format: %s", ext)
	}
}

func extractPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteByte('\n')
	}
	return buf.String(), nil
}

func extractDOCX(path string) (string, error) {
	text, err := cat.File(path)
	if err != nil {
		return "", fmt.Errorf("extract docx: %w", err)
	}
	return text, nil
}

func extractExcel(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	var buf strings.Builder
	for _, sheet := range f.GetSheetList() {
		buf.WriteString(fmt.Sprintf("## %s\n\n", sheet))
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			buf.WriteString(strings.Join(row, "\t"))
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	return buf.String(), nil
}
