package admin

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"
)

// The version covers the entire embedded bundle, including dynamic dependencies.
// Private HTML and API responses never use this public resource cache.
var adminAssetVersion = sync.OnceValue(func() string {
	hash := sha256.New()
	if err := fs.WalkDir(staticFS, "static", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := staticFS.ReadFile(name)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%d:%s:%d:", len(name), name, len(data))
		_, _ = hash.Write(data)
		return nil
	}); err != nil {
		panic(err) // Embedded files must be readable in a valid build.
	}
	return fmt.Sprintf("%x", hash.Sum(nil))[:20]
})

var adminPageTemplates sync.Map
var adminStaticAttribute = regexp.MustCompile(`(?:src|href)="/admin/static/[^"?]+"`)

func embeddedPage(name string) ([]byte, error) {
	if cached, ok := adminPageTemplates.Load(name); ok {
		return cached.([]byte), nil
	}
	data, err := staticFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	version := adminAssetVersion()
	// Share the application icon across all admin pages, including login/recovery.
	data = bytes.Replace(data, []byte("</head>"), []byte(`<link rel="icon" type="image/png" sizes="48x48" href="/admin/static/favicon.png"></head>`), 1)
	// Run the small resource loader before page scripts, preserving their order.
	data = bytes.Replace(data, []byte(`<script src=`), []byte(`<script src="/admin/static/runtime.js"></script><script src=`), 1)
	data = adminStaticAttribute.ReplaceAllFunc(data, func(attribute []byte) []byte {
		return []byte(strings.TrimSuffix(string(attribute), `"`) + `?v=` + version + `"`)
	})
	data = bytes.Replace(data, []byte("</head>"), []byte(`<meta name="admin-assets-version" content="`+version+`"></head>`), 1)
	cached, _ := adminPageTemplates.LoadOrStore(name, data)
	return cached.([]byte), nil
}

func servePrivateHTML(c *app.RequestContext, data []byte) {
	c.Response.Header.Set("Vary", "Accept-Encoding")
	if acceptsGzip(c.Request.Header.Peek("Accept-Encoding")) {
		// Never retain personalized pages in the process-wide static gzip cache.
		if compressed, err := gzipBytes(data); err == nil && len(compressed) < len(data) {
			c.Response.Header.Set("Content-Encoding", "gzip")
			c.Data(200, "text/html; charset=utf-8", compressed)
			return
		}
	}
	c.Data(200, "text/html; charset=utf-8", data)
}
