package blog

import (
	"context"
	"io"
)

// ImageStore defines an optional interface for storing and retrieving images.
// If not provided, image upload functionality will be disabled.
type ImageStore interface {
	// SaveImage stores an image and returns its URL/path for retrieval.
	// The id is a unique identifier for the image (e.g., UUID).
	// The filename is the original filename uploaded by the user.
	// The contentType is the MIME type of the image.
	// The reader provides the image data.
	SaveImage(ctx context.Context, id, filename, contentType string, reader io.Reader) (url string, err error)

	// GetImage retrieves an image by its ID.
	// Returns the content type, reader, and any error.
	GetImage(ctx context.Context, id string) (contentType string, reader io.ReadCloser, err error)

	// DeleteImage removes an image by its ID.
	DeleteImage(ctx context.Context, id string) error
}

// BlogStore defines the persistence contract the host application must satisfy.
type BlogStore interface {
	// Public methods
	GetPublishedPostBySlug(ctx context.Context, slug string) (*Post, error)
	ListPublishedPosts(ctx context.Context, limit, offset int) ([]Post, error)
	ListPostsByTag(ctx context.Context, tagSlug string, limit, offset int) ([]Post, error)

	// Admin methods
	CreatePost(ctx context.Context, p *Post) error
	UpdatePost(ctx context.Context, p *Post) error
	GetPostByID(ctx context.Context, id string) (*Post, error)
	DeletePost(ctx context.Context, id string) error
	ListAllPosts(ctx context.Context, limit, offset int) ([]Post, error)
}
