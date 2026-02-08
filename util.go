package blog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	htmd "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/google/uuid"
	"github.com/yuin/goldmark"
)

func generateID() string {
	return uuid.New().String()
}

func generateToken() string {
	return uuid.New().String()
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// markdownToHTML converts markdown content to HTML using goldmark.
func markdownToHTML(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// htmlToMarkdown converts HTML content to Markdown.
func htmlToMarkdown(html string) (string, error) {
	return htmd.ConvertString(html)
}
