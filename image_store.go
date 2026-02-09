package blog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileImageStore is a simple file-based implementation of ImageStore.
// Images are stored in a local directory and served via a configurable URL prefix.
type FileImageStore struct {
	// Directory is the local directory where images will be stored.
	Directory string
	// URLPrefix is the URL prefix for accessing images (e.g., "/uploads" or "https://cdn.example.com/images").
	URLPrefix string
}

// NewFileImageStore creates a new FileImageStore.
// If the directory doesn't exist, it will be created.
func NewFileImageStore(directory, urlPrefix string) (*FileImageStore, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create image directory: %w", err)
	}
	return &FileImageStore{
		Directory: directory,
		URLPrefix: strings.TrimSuffix(urlPrefix, "/"),
	}, nil
}

// SaveImage stores an image file and returns its URL.
func (s *FileImageStore) SaveImage(ctx context.Context, id, filename, contentType string, reader io.Reader) (string, error) {
	// Extract extension from filename or derive from content type
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = extensionFromContentType(contentType)
	}

	tempFile, err := os.CreateTemp(s.Directory, "img-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

	hasher := sha256.New()
	tee := io.TeeReader(reader, hasher)
	if _, err := io.Copy(tempFile, tee); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	hashID := hex.EncodeToString(hasher.Sum(nil))
	safeFilename := hashID + ext
	filePath := filepath.Join(s.Directory, safeFilename)

	if _, err := os.Stat(filePath); err == nil {
		_ = os.Remove(tempFile.Name())
	} else if err := os.Rename(tempFile.Name(), filePath); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to store file: %w", err)
	}

	// Store metadata in a sidecar file
	metaPath := filepath.Join(s.Directory, hashID+".meta")
	metaContent := fmt.Sprintf("%s\n%s", filename, contentType)
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		// Non-fatal: we can still serve the file
	}

	return s.URLPrefix + "/" + safeFilename, nil
}

// GetImage retrieves an image by ID.
func (s *FileImageStore) GetImage(ctx context.Context, id string) (string, io.ReadCloser, error) {
	// Try to read metadata
	baseID := id
	ext := filepath.Ext(id)
	contentType := "application/octet-stream"

	if ext != "" {
		baseID = strings.TrimSuffix(id, ext)
		contentType = contentTypeFromExtension(ext)
	}

	metaPath := filepath.Join(s.Directory, baseID+".meta")

	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		lines := strings.SplitN(string(metaBytes), "\n", 2)
		if len(lines) >= 2 {
			contentType = lines[1]
		}
	}

	// Try to open the file directly (using ID as filename)
	filePath := filepath.Join(s.Directory, id)
	file, err := os.Open(filePath)

	if err == nil {
		return contentType, file, nil
	}

	// If ID didn't have extension, try to find the file with various extensions
	if ext == "" {
		for _, tryExt := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
			tryPath := filepath.Join(s.Directory, id+tryExt)
			if f, err := os.Open(tryPath); err == nil {
				contentType = contentTypeFromExtension(tryExt)
				return contentType, f, nil
			}
		}
	}
	return "", nil, fmt.Errorf("image not found: %s", id)
}

// DeleteImage removes an image by ID.
func (s *FileImageStore) DeleteImage(ctx context.Context, id string) error {
	// Try to delete with various extensions
	deleted := false
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".meta"} {
		filePath := filepath.Join(s.Directory, id+ext)
		if err := os.Remove(filePath); err == nil {
			deleted = true
		}
	}

	if !deleted {
		return fmt.Errorf("no files found for image: %s", id)
	}
	return nil
}

func extensionFromContentType(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func contentTypeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
