package blog

import (
	"bytes"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
)

func generateID() string {
	return uuid.New().String()
}

// markdownToHTML converts markdown content to HTML using goldmark.
func markdownToHTML(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
