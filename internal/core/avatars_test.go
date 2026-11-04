package core

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/useopendesk/opendesk/internal/plan"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 30, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func (c *client) upload(method, path string, content []byte) resp {
	c.e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "picture.png")
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest(method, c.e.srv.URL+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.workspace != "" {
		req.Header.Set("OD-Workspace", c.workspace)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: raw, Header: res.Header}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func TestProfilePictureAndWorkspaceImages(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := e.signup("pic")
	wid := owner.newWorkspace("Pictures Inc")
	me := owner.do("GET", "/v1/me", nil)
	if me.Body["user"].(map[string]any)["avatar_url"] != nil {
		t.Fatalf("new user has a picture: %s", me.Raw)
	}

	if r := owner.upload("PUT", "/v1/me/avatar", []byte("<svg onload=alert(1)>")); r.Status != 422 || r.str("code") != "unsupported_image" {
		t.Fatalf("svg: %d %s", r.Status, r.Raw)
	}
	if r := owner.upload("PUT", "/v1/me/avatar", make([]byte, 6<<20)); r.Status != 413 {
		t.Fatalf("too large: %d", r.Status)
	}
	r := owner.upload("PUT", "/v1/me/avatar", testPNG(t, 200, 300))
	if r.Status != 200 || r.str("avatar_source") != "upload" || !strings.HasPrefix(r.str("avatar_url"), "http://api.test/v1/images/") {
		t.Fatalf("upload: %d %s", r.Status, r.Raw)
	}
	var audits, updates int
	_ = e.d.DB.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'user.avatar_updated'),
		(SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'member.updated')`, wid).Scan(&audits, &updates)
	if audits != 1 || updates < 1 {
		t.Fatalf("audit %d, member.updated %d", audits, updates)
	}
	members := owner.do("GET", "/v1/members", nil)
	m := members.Body["data"].([]any)[0].(map[string]any)
	if m["user"].(map[string]any)["avatar_source"] != "upload" || m["availability"] != "online" || m["presence"] != "online" {
		t.Fatalf("member: %s", members.Raw)
	}

	// The provider picture never replaces an upload.
	var uid uuid.UUID
	_ = e.d.DB.Pool.QueryRow(ctx, `SELECT user_id FROM members WHERE workspace_id = $1`, wid).Scan(&uid)
	_ = e.d.DB.Tx(ctx, func(tx pgx.Tx) error { return e.s.providerPicture(ctx, tx, uid, "https://lh3.example.com/a.png") })
	var remote string
	_ = e.d.DB.Pool.QueryRow(ctx, `SELECT avatar_remote_url FROM users WHERE id = $1`, uid).Scan(&remote)
	if remote != "" {
		t.Fatal("provider picture queued over an upload")
	}

	// Onboarding reflects the upload, then the label and the dismissal.
	ob := owner.do("GET", "/v1/onboarding", nil)
	steps := map[string]bool{}
	for _, s := range ob.Body["steps"].([]any) {
		st := s.(map[string]any)
		steps[st["id"].(string)] = st["done"].(bool)
	}
	if len(steps) != 9 || steps["setup_ai_agent"] || !steps["upload_photo"] || steps["create_label"] || steps["connect_mailbox"] || steps["try_ollie"] || ob.Body["dismissed"] != false {
		t.Fatalf("onboarding: %s", ob.Raw)
	}
	_, _ = e.d.DB.Pool.Exec(ctx, `INSERT INTO tags (workspace_id, name, color) VALUES ($1, 'Refunds', 'red')`, wid)
	_, _ = e.d.DB.Pool.Exec(ctx, `UPDATE workspaces SET settings = settings || '{"auto_assign": "round_robin"}' WHERE id = $1`, wid)
	ob = owner.do("GET", "/v1/onboarding", nil)
	if !strings.Contains(string(ob.Raw), `"id":"create_label","title":"Create a label"`) || int(ob.Body["done_count"].(float64)) != 3 {
		t.Fatalf("onboarding after: %s", ob.Raw)
	}
	if r := owner.do("POST", "/v1/onboarding/dismiss", nil); r.Status != 204 {
		t.Fatalf("dismiss: %d", r.Status)
	}
	if ob = owner.do("GET", "/v1/onboarding", nil); ob.Body["dismissed"] != true {
		t.Fatalf("dismissed: %s", ob.Raw)
	}

	if r := owner.do("DELETE", "/v1/me/avatar", nil); r.Status != 200 || r.Body["avatar_url"] != nil || r.Body["avatar_source"] != nil {
		t.Fatalf("delete: %d %s", r.Status, r.Raw)
	}

	// Workspace logo and brand images: admins only, known slots only.
	if r := owner.upload("PUT", "/v1/workspace/images/logo", testPNG(t, 400, 100)); r.Status != 200 || r.str("avatar_source") != "upload" {
		t.Fatalf("logo: %d %s", r.Status, r.Raw)
	}
	r = owner.upload("PUT", "/v1/workspace/images/help_center_favicon", testPNG(t, 64, 64))
	if r.Status != 200 || r.Body["images"].(map[string]any)["help_center_favicon"] == nil {
		t.Fatalf("favicon: %d %s", r.Status, r.Raw)
	}
	if r := owner.upload("PUT", "/v1/workspace/images/banner", testPNG(t, 64, 64)); r.Status != 422 {
		t.Fatalf("unknown slot: %d", r.Status)
	}
	var wsEvents int
	_ = e.d.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE workspace_id = $1 AND type = 'workspace.updated'`, wid).Scan(&wsEvents)
	if wsEvents < 2 {
		t.Fatalf("workspace.updated %d", wsEvents)
	}
	if r := owner.do("DELETE", "/v1/workspace/images/logo", nil); r.Status != 200 || r.Body["avatar_url"] != nil {
		t.Fatalf("remove logo: %d %s", r.Status, r.Raw)
	}
}

func TestEffectiveMailStateOnWorkspaceMembershipAndImageEvent(t *testing.T) {
	e := newEnv(t)
	e.d.Cfg.Collab = true
	e.d.Plan = plan.New(false, 1)
	owner, _ := e.signup("effective-mail")
	wid := owner.newWorkspace("Effective Mail Co")
	owner.workspace = wid

	check := func(w map[string]any, available bool, reason string) {
		t.Helper()
		if w["mail_available"] != available {
			t.Fatalf("mail availability: wanted %v, got %#v", available, w)
		}
		if reason == "" {
			if _, ok := w["mail_disabled_reason"]; ok {
				t.Fatalf("unexpected disabled reason: %#v", w["mail_disabled_reason"])
			}
		} else if w["mail_disabled_reason"] != reason {
			t.Fatalf("mail reason: wanted %q, got %#v", reason, w["mail_disabled_reason"])
		}
	}
	checkWorkspace := func(available bool, reason string) {
		t.Helper()
		current := owner.do("GET", "/v1/workspace", nil)
		if current.Status != 200 {
			t.Fatalf("GET workspace: %d %s", current.Status, current.Raw)
		}
		check(current.Body, available, reason)
		me := owner.do("GET", "/v1/me", nil)
		if me.Status != 200 {
			t.Fatalf("GET me: %d %s", me.Status, me.Raw)
		}
		memberships := me.Body["memberships"].([]any)
		for _, raw := range memberships {
			membership := raw.(map[string]any)
			workspace := membership["workspace"].(map[string]any)
			if workspace["id"] == wid {
				check(workspace, available, reason)
				return
			}
		}
		t.Fatalf("workspace %s missing from /v1/me: %s", wid, me.Raw)
	}
	checkWorkspace(true, "")
	e.d.Plan.CollabOff = true
	checkWorkspace(false, "instance_disabled")
	e.d.Plan.CollabOff = false
	originalPlan := e.d.Plan
	e.d.Plan = plan.New(true, 1)
	checkWorkspace(false, "plan")
	e.d.Plan = originalPlan
	e.d.Cfg.Collab = false
	checkWorkspace(false, "instance_disabled")
	e.d.Cfg.Collab = true

	if r := owner.do("PATCH", "/v1/workspace", map[string]any{"settings": map[string]any{"mail_enabled": false}}); r.Status != 200 {
		t.Fatalf("disable Mail: %d %s", r.Status, r.Raw)
	}
	checkWorkspace(false, "workspace_disabled")

	if r := owner.upload("PUT", "/v1/workspace/images/logo", testPNG(t, 40, 20)); r.Status != 200 {
		t.Fatalf("upload logo: %d %s", r.Status, r.Raw)
	}
	var reason string
	if err := e.d.DB.Pool.QueryRow(context.Background(), `SELECT data->'workspace'->>'mail_disabled_reason'
		FROM events WHERE workspace_id = $1 AND type = 'workspace.updated' ORDER BY seq DESC LIMIT 1`, wid).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "workspace_disabled" {
		t.Fatalf("workspace image event mail reason: %q", reason)
	}
}

func TestAvailabilityPermissions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := e.signup("boss")
	wid := owner.newWorkspace("Availability Co")
	agent, _ := e.signup("worker")
	viewer, _ := e.signup("watcher")
	ids := map[*client]string{}
	for c, role := range map[*client]string{agent: "agent", viewer: "viewer"} {
		me := c.do("GET", "/v1/me", nil)
		id := me.Body["user"].(map[string]any)["id"].(string)
		ids[c] = id
		if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO members (workspace_id, user_id, role) VALUES ($1, $2, $3)`, wid, id, role); err != nil {
			t.Fatal(err)
		}
		c.workspace = wid
	}
	if r := agent.do("PATCH", "/v1/members/me/availability", map[string]any{"availability": "away"}); r.Status != 200 || r.str("availability") != "away" {
		t.Fatalf("self: %d %s", r.Status, r.Raw)
	}
	if r := agent.do("PATCH", "/v1/members/me/availability", map[string]any{"max_open_tickets": 3}); r.Status != 403 {
		t.Fatalf("agent cap: %d", r.Status)
	}
	if r := agent.do("PATCH", "/v1/members/"+ids[viewer]+"/availability", map[string]any{"availability": "offline"}); r.Status != 403 {
		t.Fatalf("agent sets another: %d", r.Status)
	}
	if r := owner.do("PATCH", "/v1/members/"+ids[agent]+"/availability", map[string]any{"max_open_tickets": 5}); r.Status != 200 || r.Body["max_open_tickets"].(float64) != 5 {
		t.Fatalf("owner cap: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("PATCH", "/v1/members/"+ids[agent]+"/availability", map[string]any{"max_open_tickets": nil}); r.Status != 200 || r.Body["max_open_tickets"] != nil {
		t.Fatalf("clear cap: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("PATCH", "/v1/members/"+ids[viewer]+"/availability", map[string]any{"max_open_tickets": 5}); r.Status != 422 {
		t.Fatalf("viewer cap: %d", r.Status)
	}
	if r := owner.do("PATCH", "/v1/members/"+ids[agent]+"/availability", map[string]any{"max_open_tickets": 0}); r.Status != 422 {
		t.Fatalf("zero cap: %d", r.Status)
	}
	if r := agent.do("POST", "/v1/me/heartbeat", nil); r.Status != 204 {
		t.Fatalf("heartbeat: %d", r.Status)
	}
	// Auto-away: online but unseen counts as away in presence.
	_, _ = e.d.DB.Pool.Exec(ctx, `UPDATE workspaces SET settings = settings || '{"auto_away_minutes": 30}' WHERE id = $1`, wid)
	_, _ = e.d.DB.Pool.Exec(ctx, `UPDATE members SET availability = 'online' WHERE workspace_id = $1`, wid)
	_, _ = e.d.DB.Pool.Exec(ctx, `UPDATE users SET last_seen_at = now() - interval '1 hour' WHERE id = $1`, ids[agent])
	list := owner.do("GET", "/v1/members", nil)
	for _, raw := range list.Body["data"].([]any) {
		m := raw.(map[string]any)
		if m["user"].(map[string]any)["id"] == ids[agent] && m["presence"] != "away" {
			t.Fatalf("presence %v", m["presence"])
		}
	}
}

// TestMemberMessageDelete checks an admin sets an agent's own delete rule, back to the
// workspace's with default, and that only agents take one.
func TestMemberMessageDelete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner, _ := e.signup("deleter-boss")
	wid := owner.newWorkspace("Delete Rules Co")
	agent, _ := e.signup("deleter-agent")
	viewer, _ := e.signup("deleter-viewer")
	ids := map[*client]string{}
	for c, role := range map[*client]string{agent: "agent", viewer: "viewer"} {
		me := c.do("GET", "/v1/me", nil)
		id := me.Body["user"].(map[string]any)["id"].(string)
		ids[c] = id
		if _, err := e.d.DB.Pool.Exec(ctx, `INSERT INTO members (workspace_id, user_id, role) VALUES ($1, $2, $3)`, wid, id, role); err != nil {
			t.Fatal(err)
		}
		c.workspace = wid
	}
	path := "/v1/members/" + ids[agent] + "/permissions"
	if r := agent.do("PATCH", path, map[string]any{"message_delete": "all"}); r.Status != 403 {
		t.Fatalf("agent sets their own: %d", r.Status)
	}
	if r := owner.do("PATCH", path, map[string]any{"message_delete": "all"}); r.Status != 200 || r.str("message_delete") != "all" {
		t.Fatalf("owner sets: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("PATCH", path, map[string]any{"message_delete": "default"}); r.Status != 200 || r.Body["message_delete"] != nil {
		t.Fatalf("back to the workspace's: %d %s", r.Status, r.Raw)
	}
	if r := owner.do("PATCH", path, map[string]any{"message_delete": "everyone"}); r.Status != 422 {
		t.Fatalf("unknown value: %d", r.Status)
	}
	if r := owner.do("PATCH", "/v1/members/"+ids[viewer]+"/permissions", map[string]any{"message_delete": "own"}); r.Status != 422 {
		t.Fatalf("viewer: %d", r.Status)
	}
	var n int
	if err := e.d.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE workspace_id = $1 AND action = 'member.permissions_changed'`, wid).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("audit entries %d", n)
	}
}
