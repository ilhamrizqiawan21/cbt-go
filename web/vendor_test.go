package web_test

import (
	"io/fs"
	"strings"
	"testing"

	"cbt-go/web"
)

// Bootstrap & ikon disajikan dari binary (DECISION G-019) agar tampilan tidak rusak
// saat internet sekolah putus. Tes ini mencegah referensi CDN muncul kembali.
func TestAsetVendorLokal(t *testing.T) {
	for _, p := range []string{
		"static/css/vendor/bootstrap.min.css",
		"static/css/vendor/bootstrap-icons.min.css",
		"static/css/vendor/fonts/bootstrap-icons.woff2",
		"static/js/vendor/bootstrap.bundle.min.js",
	} {
		if _, err := fs.Stat(web.FS, p); err != nil {
			t.Errorf("aset vendor %s tidak ter-embed: %v", p, err)
		}
	}

	files, err := fs.Glob(web.FS, "templates/*.gohtml")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		b, err := fs.ReadFile(web.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "cdn.jsdelivr.net") {
			t.Errorf("%s masih memuat aset dari cdn.jsdelivr.net", f)
		}
	}
}
