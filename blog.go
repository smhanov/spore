package blog

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:generate sh -c "cd frontend && npm install && npm run build"

//go:embed templates/*.html
var defaultTemplatesFS embed.FS

//go:embed frontend/dist frontend/dist/* frontend/dist/**
var adminAssetsFS embed.FS

// Config controls how the blog package integrates with the host application.
type Config struct {
	Store               BlogStore
	ImageStore          ImageStore // Optional: enables image upload functionality
	RoutePrefix         string
	AdminAuthMiddleware func(http.Handler) http.Handler
	LayoutTemplatePath  string
	CustomCSSURLs       []string
	// StaticFilePath is the optional directory from which to serve files not found as posts.
	StaticFilePath string
	// TemplatesDir is an optional directory containing custom templates (list.html, post.html).
	// If set, templates found here override the embedded defaults.
	TemplatesDir string
	// ListAll disables pagination and displays every published post on a single page.
	ListAll bool
	// Optional metadata used for WXR export/import.
	SiteTitle string
	SiteDescription          string
	SiteURL                  string
	SiteLanguage             string
	DefaultAuthorLogin       string
	DefaultAuthorDisplayName string
	ImportAuthorID           int
}

type service struct {
	cfg         Config
	templates   map[string]*template.Template
	routePrefix string
	adminFS     fs.FS
	tasks       *taskRunner
	store       *storeAdapter
}

// Handler serves the blog's HTTP routes and provides methods for integrating
// blog data (such as sitemap entries) into the host application.
type Handler struct {
	http.Handler
	svc *service
}

// NewHandler wires routes for public and admin surfaces using the supplied configuration.
func NewHandler(cfg Config) (*Handler, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if err := cfg.Store.Migrate(context.Background()); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	routePrefix := cfg.RoutePrefix
	if routePrefix == "" {
		routePrefix = "/blog"
	}
	if !strings.HasPrefix(routePrefix, "/") {
		routePrefix = "/" + routePrefix
	}
	tpls, err := parseTemplates(cfg)
	if err != nil {
		return nil, err
	}

	s := &service{
		cfg:         cfg,
		templates:   tpls,
		routePrefix: strings.TrimSuffix(routePrefix, "/"),
		adminFS:     adminAssetsFS,
		store:       newStoreAdapter(cfg.Store),
	}

	r := chi.NewRouter()

	r.Route(s.routePrefix, func(r chi.Router) {
		s.mountPublicRoutes(r)

		// Admin assets and API
		adminRouter := chi.NewRouter()
		if cfg.AdminAuthMiddleware != nil {
			adminRouter.Use(cfg.AdminAuthMiddleware)
		}
		s.mountAdminRoutes(adminRouter)
		r.Mount("/admin", adminRouter)
	})

	// Start background task runner (resumes pending tasks from DB)
	s.tasks = newTaskRunner(s)
	s.tasks.start()

	return &Handler{Handler: r, svc: s}, nil
}

func parseTemplates(cfg Config) (map[string]*template.Template, error) {
	funcMap := template.FuncMap{
		"safeHTML":            func(s string) template.HTML { return template.HTML(s) },
		"formatPublishedDate": formatPublishedDate,
		"rfc3339": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format(time.RFC3339)
		},
		"jsonEscape": func(s string) string {
			b, _ := json.Marshal(s)
			// Strip the surrounding quotes since the template already provides them
			return string(b[1 : len(b)-1])
		},
		"truncate":  tplTruncate,
		"stripHTML": tplStripHTML,
		"now":       func() time.Time { return time.Now() },
	}

	build := func(extra ...string) (*template.Template, error) {
		var baseTpl *template.Template
		if cfg.LayoutTemplatePath != "" {
			content, err := os.ReadFile(cfg.LayoutTemplatePath)
			if err != nil {
				return nil, fmt.Errorf("read layout template: %w", err)
			}
			baseTpl, err = template.New(path.Base(cfg.LayoutTemplatePath)).Funcs(funcMap).Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("parse custom layout: %w", err)
			}
		} else {
			var err error
			baseTpl, err = template.New("base.html").Funcs(funcMap).ParseFS(defaultTemplatesFS, "templates/base.html")
			if err != nil {
				return nil, err
			}
		}

		clone, err := baseTpl.Clone()
		if err != nil {
			return nil, err
		}
		// Always include comments.html partial.
		allPatterns := append(extra, "templates/comments.html")
		if _, err := clone.ParseFS(defaultTemplatesFS, allPatterns...); err != nil {
			return nil, err
		}
		return clone, nil
	}

	loadTemplate := func(name string) (string, bool) {
		if cfg.TemplatesDir != "" {
			filePath := path.Join(cfg.TemplatesDir, name)
			data, err := os.ReadFile(filePath)
			if err == nil {
				return string(data), true
			}
		}
		return "", false
	}

	buildTpl := func(name string) (*template.Template, error) {
		if content, ok := loadTemplate(name); ok {
			var baseTpl *template.Template
			if cfg.LayoutTemplatePath != "" {
				baseContent, err := os.ReadFile(cfg.LayoutTemplatePath)
				if err != nil {
					return nil, fmt.Errorf("read layout template: %w", err)
				}
				baseTpl, err = template.New(path.Base(cfg.LayoutTemplatePath)).Funcs(funcMap).Parse(string(baseContent))
				if err != nil {
					return nil, fmt.Errorf("parse custom layout: %w", err)
				}
			} else if baseContent, ok := loadTemplate("base.html"); ok {
				var err error
				baseTpl, err = template.New("base.html").Funcs(funcMap).Parse(baseContent)
				if err != nil {
					return nil, fmt.Errorf("parse custom base: %w", err)
				}
			} else {
				var err error
				baseTpl, err = template.New("base.html").Funcs(funcMap).ParseFS(defaultTemplatesFS, "templates/base.html")
				if err != nil {
					return nil, err
				}
			}
			clone, err := baseTpl.Clone()
			if err != nil {
				return nil, err
			}
			// Include comments.html partial (custom override or embedded default).
			if commentsContent, ok := loadTemplate("comments.html"); ok {
				if _, err := clone.Parse(commentsContent); err != nil {
					return nil, fmt.Errorf("parse custom comments.html: %w", err)
				}
			} else {
				if _, err := clone.ParseFS(defaultTemplatesFS, "templates/comments.html"); err != nil {
					return nil, fmt.Errorf("parse embedded comments.html: %w", err)
				}
			}
			if _, err := clone.Parse(content); err != nil {
				return nil, fmt.Errorf("parse custom %s: %w", name, err)
			}
			return clone, nil
		}
		return build("templates/" + name)
	}

	listTpl, err := buildTpl("list.html")
	if err != nil {
		return nil, err
	}
	postTpl, err := buildTpl("post.html")
	if err != nil {
		return nil, err
	}

	return map[string]*template.Template{
		"list.html": listTpl,
		"post.html": postTpl,
	}, nil
}
