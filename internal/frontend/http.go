package frontend

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/scribble-rs/scribble.rs/internal/translations"
)

var (
	//go:embed templates/*
	templateFS    embed.FS
	pageTemplates *template.Template

	//go:embed resources/*
	frontendResourcesFS embed.FS
)

func init() {
	var err error
	pageTemplates, err = template.ParseFS(templateFS, "templates/*")
	if err != nil {
		panic(fmt.Errorf("error loading templates: %w", err))
	}
}

// BasePageConfig is data that all pages require to function correctly, no matter
// whether error page or lobby page.
type BasePageConfig struct {
	// Version is the tagged source code version of this build. Can be empty for dev
	// builds. Untagged commits will be of format `tag-N-gSHA`.
	Version string `json:"version"`
	// Commit that was deployed, if we didn't deploy a concrete tag.
	Commit string `json:"commit"`
	// RootPath is the path directly after the domain and before the
	// scribble.rs paths. For example if you host scribblers on painting.com
	// but already host a different website, then your API paths might have to
	// look like this: painting.com/scribblers/v1.
	RootPath string `json:"rootPath"`
	// RootURL is similar to RootPath, but contains only the protocol and
	// domain. So it could be https://painting.com. This is required for some
	// non critical functionality, such as metadata tags.
	RootURL string `json:"rootUrl"`
	// CacheBust is a string that is appended to all resources to prevent
	// browsers from using cached data of a previous version, but still have
	// long lived max age values.
	CacheBust string `json:"cacheBust"`
}

// SetupRoutes registers the official webclient endpoints with the http package.
func (handler *SSRHandler) SetupRoutes(register func(string, string, http.HandlerFunc)) {
	var genericFileHandler http.HandlerFunc
	if dir := handler.cfg.ServeDirectories[""]; dir != "" {
		delete(handler.cfg.ServeDirectories, "")
		fileServer := http.FileServer(http.FS(os.DirFS(dir)))
		genericFileHandler = fileServer.ServeHTTP
	}

	for route, dir := range handler.cfg.ServeDirectories {
		fileServer := http.FileServer(http.FS(os.DirFS(dir)))
		fmt.Println(filepath.Base(dir), fileServer)
		fileHandler := http.StripPrefix(
			"/"+path.Join(handler.cfg.RootPath, route)+"/",
			http.HandlerFunc(fileServer.ServeHTTP),
		).ServeHTTP
		register(
			// Trailing slash means wildcard.
			"GET", path.Join(handler.cfg.RootPath, route)+"/",
			fileHandler,
		)
	}

	register("GET", handler.cfg.RootPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || r.URL.Path == "/" {
			handler.indexPageHandler(w, r)
			return
		}

		if genericFileHandler != nil {
			genericFileHandler.ServeHTTP(w, r)
		}
	})

	fileServer := http.FileServer(http.FS(frontendResourcesFS))
	register(
		"GET", path.Join(handler.cfg.RootPath, "resources", "{file}"),
		http.StripPrefix(
			"/"+handler.cfg.RootPath,
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Duration of 1 year, since we use cachebusting anyway.
				w.Header().Set("Cache-Control", "public, max-age=31536000")

				fileServer.ServeHTTP(w, r)
			}),
		).ServeHTTP,
	)
	register("GET", path.Join(handler.cfg.RootPath, "ssrEnterLobby", "{lobby_id}"), handler.ssrEnterLobby)
	register("POST", path.Join(handler.cfg.RootPath, "ssrCreateLobby"), handler.ssrCreateLobby)
}

// errorPageData represents the data that error.html requires to be displayed.
type errorPageData struct {
	*BasePageConfig
	// ErrorMessage displayed on the page.
	ErrorMessage string

	Translation translations.Translation
	Locale      string
}

// userFacingError will return the occurred error as a custom html page to the caller.
func (handler *SSRHandler) userFacingError(w http.ResponseWriter, errorMessage string) {
	err := pageTemplates.ExecuteTemplate(w, "error-page", &errorPageData{
		BasePageConfig: handler.basePageConfig,
		ErrorMessage:   errorMessage,
	})
	// This should never happen, but if it does, something is very wrong.
	if err != nil {
		panic(err)
	}
}
