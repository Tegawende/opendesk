package supportchannels

import (
	"net/http"
	"testing"

	"github.com/useopendesk/opendesk/internal/helpdesk/hdtest"
	"github.com/useopendesk/opendesk/internal/ids"
)

func TestPriorityDestinationAPI(t *testing.T) {
	e := hdtest.New(t, Register)
	ws := e.NewWorkspace()
	connectionID := ids.New()
	credentials, err := e.D.Box.SealJSON(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	e.Exec(`INSERT INTO support_channel_connections(id,workspace_id,provider,name,external_account_id,credentials_enc)
		VALUES($1,$2,'slack','Support','T1',$3)`, connectionID, ws.ID, credentials)

	path := "/v1/support-channels/destinations"
	res := e.Do(http.MethodPost, path, ws.Admin.As(ws), map[string]any{
		"connection_id": connectionID.String(), "priority": "urgent", "external_channel_id": "C1",
	}).Want(t, http.StatusCreated)
	var created destination
	res.JSON(t, &created)
	if created.Priority == nil || *created.Priority != "urgent" {
		t.Fatalf("created priority = %v, want urgent", created.Priority)
	}
	e.Do(http.MethodPost, path, ws.Admin.As(ws), map[string]any{
		"connection_id": connectionID.String(), "priority": "urgent", "external_channel_id": "C2",
	}).Want(t, http.StatusConflict)
	e.Do(http.MethodPost, path, ws.Admin.As(ws), map[string]any{
		"connection_id": connectionID.String(), "priority": "critical", "external_channel_id": "C3",
	}).Want(t, http.StatusUnprocessableEntity)

	var listed struct {
		Data []destination `json:"data"`
	}
	e.Do(http.MethodGet, path, ws.Admin.As(ws), nil).Want(t, http.StatusOK).JSON(t, &listed)
	if len(listed.Data) != 1 || listed.Data[0].Priority == nil || *listed.Data[0].Priority != "urgent" {
		t.Fatalf("destinations = %+v", listed.Data)
	}
}
