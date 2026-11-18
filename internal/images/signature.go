package images

// Pictures in email signatures. A member uploads a picture from a signature editor and gets a
// public, stable URL for its <img>: the email clients of the people who receive the signature
// load it from there. It is stored and served like logos and profile pictures (the images
// table, the file storage, GET /v1/images/{id}: public, unguessable, cached), with two
// differences:
//   - it keeps its shape: a wide logo is not cropped or padded to a square, only made smaller;
//   - it is never replaced or collected (Collect keeps the signature slot), since the emails
//     already sent point at it. It goes with its workspace.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"golang.org/x/image/draw"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/deps"
	"github.com/useopendesk/opendesk/internal/domain"
	"github.com/useopendesk/opendesk/internal/httpx"
)

const (
	// SlotSignature is the slot of a member's signature pictures (owner "user", in a workspace).
	SlotSignature = "signature"
	// SignatureMaxSide bounds the stored picture: twice the widest size the editor inserts
	// (300 px), so it stays sharp on high-density screens.
	SignatureMaxSide = 600
	// MaxSignatureImages is how many signature pictures a member keeps in a workspace.
	MaxSignatureImages = 200
)

// SignatureImage is an uploaded signature picture.
type SignatureImage struct {
	URL    string `json:"url" format:"uri" doc:"Public, stable URL of the picture (a PNG), for an <img> in a signature"`
	Width  int    `json:"width" doc:"Its width in pixels (at most 600)"`
	Height int    `json:"height" doc:"Its height in pixels (at most 600)"`
}

// RenderSignature makes a decoded picture at most SignatureMaxSide on its longer side (never
// larger), keeping its shape. Large is that picture and Width and Height its size; Small is a
// 64 px one, so the objects sit at the keys every image has (Keys), where the workspace purge
// and Discard find them.
func RenderSignature(img image.Image) (Processed, error) {
	big := fit(img, SignatureMaxSide)
	large, err := encode(big)
	if err != nil {
		return Processed{}, err
	}
	small, err := encode(fit(img, SmallSize))
	if err != nil {
		return Processed{}, err
	}
	sum := sha256.Sum256(large)
	b := big.Bounds()
	return Processed{Large: large, Small: small, SHA256: hex.EncodeToString(sum[:]), Width: b.Dx(), Height: b.Dy()}, nil
}

// fit scales img so its longer side is at most side (never up), with a quality filter: a
// signature picture is shown at about its stored size.
func fit(img image.Image, side int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := w, h
	if w > side || h > side {
		if w > h {
			tw, th = side, max(1, side*h/w)
		} else {
			tw, th = max(1, side*w/h), side
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, tw, th))
	if tw == w && th == h {
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		return dst
	}
	src := shrink(img, 4*side) // bounds the resampling work for large inputs
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// RegisterSignature adds POST /v1/signature-images.
func RegisterSignature(d *deps.Deps, api huma.API) {
	huma.Register(api, UploadOperation(api, huma.Operation{OperationID: "upload-signature-image", Method: http.MethodPost,
		Path: "/v1/signature-images", Summary: "Upload a picture for a signature", Tags: []string{"Account"},
		DefaultStatus: http.StatusCreated,
		Description: "PNG, JPEG, GIF or WebP, at most 5 MB. Stored as a PNG of at most 600 px on its longer side, its shape kept, " +
			"and served without authentication from the returned URL, for an <img> in a personal, team or mailbox signature. " +
			"The picture stays as long as the workspace: emails already sent show it."}),
		func(ctx context.Context, in *UploadInput) (*struct{ Body SignatureImage }, error) {
			p, err := auth.RequireWorkspace(ctx, auth.RoleViewer, "")
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if p.Kind != auth.KindUser {
				return nil, httpx.Forbidden()
			}
			if !d.Limiter.Allow("signature-image:"+p.UserID.String(), 60, time.Hour) {
				return nil, httpx.RateLimited()
			}
			var n int
			if err := d.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM images WHERE workspace_id = $1 AND owner_type = $2 AND owner_id = $3 AND slot = $4`,
				p.WorkspaceID, OwnerUser, p.UserID, SlotSignature).Scan(&n); err != nil {
				return nil, httpx.Wrap(err)
			}
			if n >= MaxSignatureImages {
				return nil, httpx.E(http.StatusUnprocessableEntity, "too_many_images", "You have uploaded the most signature pictures a member can keep.")
			}
			img, err := readSignatureUpload(in)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			wid := p.WorkspaceID
			o := Owner{Type: OwnerUser, ID: p.UserID, WorkspaceID: &wid, Slot: SlotSignature}
			id, prefix, err := Put(ctx, d.Storage, o, img)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if err := d.DB.Tx(ctx, func(tx pgx.Tx) error { return Insert(ctx, tx, o, id, prefix, "upload", img.SHA256) }); err != nil {
				Discard(d.Storage, prefix)
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body SignatureImage }{SignatureImage{URL: domain.ImageURL(id), Width: img.Width, Height: img.Height}}, nil
		})
}

// readSignatureUpload is ReadUpload with the signature rendering.
func readSignatureUpload(in *UploadInput) (Processed, error) {
	defer removeForm(in.RawBody.Form)
	data := in.RawBody.Data()
	if data == nil || data.File.File == nil || !data.File.IsSet {
		return Processed{}, httpx.Validation("file is required")
	}
	defer data.File.Close()
	if data.File.Size > MaxBytes {
		return Processed{}, ClientError(ErrTooLarge)
	}
	b, err := ReadLimited(data.File.File, MaxBytes)
	if err != nil {
		return Processed{}, ClientError(err)
	}
	img, err := Decode(b)
	if err != nil {
		return Processed{}, ClientError(err)
	}
	p, err := RenderSignature(img)
	if err != nil {
		return Processed{}, ClientError(err)
	}
	return p, nil
}
