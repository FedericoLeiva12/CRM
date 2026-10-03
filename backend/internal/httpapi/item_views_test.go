package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"siracrm/internal/domain"
	"siracrm/internal/store"
)

func TestSectionItemViews(t *testing.T) {
	ctx := context.Background()
	pool, handler, cookie := startCRM(t, ctx)
	defer pool.Close()
	repository := store.New(pool)
	getViews := func(sectionID string) []domain.SectionView {
		t.Helper()
		sections, err := repository.ListSections(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, section := range sections {
			if section.ID == sectionID {
				return section.Views
			}
		}
		t.Fatalf("section %s missing", sectionID)
		return nil
	}
	info := domain.SectionView{ID: "info", Enabled: true, Config: map[string]any{}}
	activity := domain.SectionView{ID: "activity", Enabled: true, Config: map[string]any{}}
	save := func(sectionID string, views []domain.SectionView, status int) {
		t.Helper()
		expectStatus(t, apiCall(t, handler, cookie, "", "PUT", "/api/sections/"+sectionID+"/views", map[string]any{"views": views}), status)
	}
	if views := getViews("clients"); len(views) != 1 || views[0].ID != "info" || !views[0].Enabled {
		t.Fatalf("existing section has no default Info: %+v", views)
	}
	expectStatus(t, apiCall(t, handler, cookie, "", "POST", "/api/sections", map[string]string{"id": "employees", "name": "Employees"}), 201)
	if views := getViews("employees"); len(views) != 1 || !views[0].Enabled {
		t.Fatal("new section missing Info", views)
	}
	expectStatus(t, apiCall(t, handler, "", "", "GET", "/api/item-view-types", nil), 401)
	expectStatus(t, apiCall(t, handler, cookie, "", "GET", "/api/item-view-types", nil), 200)
	save("clients", []domain.SectionView{info, activity}, 200)
	if views := getViews("clients"); len(views) != 2 || views[1].ID != "activity" || !views[1].Enabled {
		t.Fatal("configuration not persisted", views)
	}
	if views := getViews("prospects"); len(views) != 1 {
		t.Fatal("configuration leaked to another section", views)
	}
	save("clients", []domain.SectionView{activity}, 400)
	save("clients", []domain.SectionView{{ID: "info", Enabled: false}}, 400)
	save("clients", []domain.SectionView{info, {ID: "unknown", Enabled: true}}, 400)
	if views := getViews("clients"); len(views) != 2 {
		t.Fatal("rejected update changed stored views", views)
	}
	activity.Enabled = false
	save("clients", []domain.SectionView{info, activity}, 200)
	if views := getViews("clients"); len(views) != 2 || views[1].Enabled {
		t.Fatal("disable not persisted", views)
	}
	save("clients", []domain.SectionView{info}, 200)
	if views := getViews("clients"); len(views) != 1 {
		t.Fatal("optional view not removed", views)
	}
	save("missing", []domain.SectionView{info}, 404)
	invite := apiCall(t, handler, cookie, "", "POST", "/api/invites", map[string]string{"email": "member@example.test", "role": "member"})
	expectStatus(t, invite, 201)
	var invitation map[string]string
	if err := json.Unmarshal(invite.Body.Bytes(), &invitation); err != nil {
		t.Fatal(err)
	}
	accepted := apiCall(t, handler, "", "", "POST", "/api/invite/"+invitation["token"], map[string]string{"name": "Member", "password": "member-long-password"})
	expectStatus(t, accepted, 201)
	memberCookie := accepted.Result().Cookies()[0].Name + "=" + accepted.Result().Cookies()[0].Value
	expectStatus(t, apiCall(t, handler, memberCookie, "", "GET", "/api/item-view-types", nil), 200)
	expectStatus(t, apiCall(t, handler, memberCookie, "", "PUT", "/api/sections/clients/views", map[string]any{"views": []domain.SectionView{info, activity}}), 403)
	var auditCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit WHERE action='configure_item_views'").Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("configuration audit missing: %d %v", auditCount, err)
	}
}
