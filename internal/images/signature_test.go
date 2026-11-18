package images_test

// Uploading a signature picture, serving it to anyone, and keeping it. Needs the test
// database (hdtest).

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/images"
)

func TestMain(m *testing.M) { hdtest.Main(m) }

func signaturePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x), 90, uint8(y), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngSize(t *testing.T, b []byte) (int, int) {
	t.Helper()
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Width, cfg.Height
}

func TestRenderSignatureKeepsShape(t *testing.T) {
	for _, c := range []struct{ w, h, wantW, wantH, smallW, smallH int }{
		{1200, 300, 600, 150, 64, 16}, // a wide logo: smaller, not squared
		{300, 900, 200, 600, 21, 64},  // a tall picture
		{120, 40, 120, 40, 64, 21},    // a small one is never made larger
	} {
		img, err := images.Decode(signaturePNG(t, c.w, c.h))
		if err != nil {
			t.Fatal(err)
		}
		p, err := images.RenderSignature(img)
		if err != nil {
			t.Fatal(err)
		}
		if w, h := pngSize(t, p.Large); w != c.wantW || h != c.wantH || p.Width != c.wantW || p.Height != c.wantH {
			t.Errorf("%dx%d: large %dx%d (reported %dx%d), want %dx%d", c.w, c.h, w, h, p.Width, p.Height, c.wantW, c.wantH)
		}
		if w, h := pngSize(t, p.Small); w != c.smallW || h != c.smallH {
			t.Errorf("%dx%d: small %dx%d, want %dx%d", c.w, c.h, w, h, c.smallW, c.smallH)
		}
	}
}

func TestSignatureImageUpload(t *testing.T) {
	e := hdtest.New(t, func(d *deps.Deps, api huma.API, r chi.Router) {
		images.RegisterSignature(d, api)
		images.Register(d, r)
	})
	ws := e.NewWorkspace()
	var got images.SignatureImage

	// Any member, a viewer too (they write their own signature); not without signing in.
	e.Upload("/v1/signature-images", hdtest.Auth{}, "logo.png", signaturePNG(t, 64, 64)).Want(t, 401)
	e.Upload("/v1/signature-images", ws.Viewer.As(ws), "logo.png", signaturePNG(t, 64, 64)).Want(t, 201)
	r := e.Upload("/v1/signature-images", ws.Agent.As(ws), "a.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)).Want(t, 422)
	if !strings.Contains(string(r.Body), "unsupported_image") {
		t.Fatalf("svg: %s", r.Body)
	}

	e.Upload("/v1/signature-images", ws.Agent.As(ws), "banner.png", append(signaturePNG(t, 1200, 300), []byte("<script>x</script>")...)).Want(t, 201).JSON(t, &got)
	if got.Width != 600 || got.Height != 150 {
		t.Fatalf("size %+v", got)
	}
	i := strings.Index(got.URL, "/v1/images/")
	if !strings.HasPrefix(got.URL, "http://api.test/v1/images/") || i < 0 {
		t.Fatalf("url %q", got.URL)
	}
	path := got.URL[i:]

	// Served to anyone (a recipient's mail client), as a fresh PNG of its own shape, cached for good.
	get := func() (*http.Response, []byte) {
		res, err := http.Get(e.Srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, b
	}
	res, b := get()
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/png" || !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("served %d %v", res.StatusCode, res.Header)
	}
	if bytes.Contains(b, []byte("script")) {
		t.Fatal("payload served")
	}
	if w, h := pngSize(t, b); w != 600 || h != 150 {
		t.Fatalf("served %dx%d", w, h)
	}

	// The sweep of pictures no one shows keeps it: emails already sent point at it.
	e.Exec(`UPDATE images SET created_at = now() - interval '2 hours' WHERE workspace_id = $1`, ws.ID)
	if err := images.Collect(e.Ctx, e.D); err != nil {
		t.Fatal(err)
	}
	if res, _ := get(); res.StatusCode != 200 {
		t.Fatalf("collected: %d", res.StatusCode)
	}
	var n int
	e.Scalar(`SELECT count(*) FROM images WHERE workspace_id = $1 AND owner_type = 'user' AND owner_id = $2 AND slot = 'signature'`, &n, ws.ID, ws.Agent.ID)
	if n != 1 {
		t.Fatalf("%d signature images for the agent", n)
	}

	// A member keeps a bounded number of them.
	e.Exec(`INSERT INTO images (id, workspace_id, owner_type, owner_id, slot, source, storage_key, sha256)
		SELECT gen_random_uuid(), $1, 'user', $2, 'signature', 'upload', 'x', 'x' FROM generate_series(1, $3::int)`, ws.ID, ws.Admin.ID, images.MaxSignatureImages)
	r = e.Upload("/v1/signature-images", ws.Admin.As(ws), "logo.png", signaturePNG(t, 64, 64)).Want(t, 422)
	if !strings.Contains(string(r.Body), "too_many_images") {
		t.Fatalf("cap: %s", r.Body)
	}
}
