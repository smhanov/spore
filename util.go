package blog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	htmd "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
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
	return markdownToHTMLWithOptions(markdown, false)
}

// markdownToHTMLUnsafe converts markdown content to HTML and allows raw HTML passthrough.
func markdownToHTMLUnsafe(markdown string) (string, error) {
	return markdownToHTMLWithOptions(markdown, true)
}

func markdownToHTMLWithOptions(markdown string, allowUnsafe bool) (string, error) {
	var buf bytes.Buffer
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
		),
	)
	if allowUnsafe {
		md = goldmark.New(
			goldmark.WithExtensions(
				extension.Table,
			),
			goldmark.WithRendererOptions(
				gmhtml.WithUnsafe(),
			),
		)
	}
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// htmlToMarkdown converts HTML content to Markdown.
func htmlToMarkdown(html string) (string, error) {
	return htmd.ConvertString(html)
}
