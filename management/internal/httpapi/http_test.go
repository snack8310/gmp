package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/snack8310/gmp/management/internal/httpapi"
	"github.com/snack8310/gmp/management/internal/management"
)

type client struct {
	t *testing.T
	h http.Handler
}

func newClient(t *testing.T) *client {
	t.Helper()
	return &client{t: t, h: httpapi.NewHandler(management.NewStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))}
}

func (c *client) call(method, path, body string, want int) map[string]any {
	c.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	if w.Code != want {
		c.t.Fatalf("%s %s: status %d, want %d; body %s", method, path, w.Code, want, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		c.t.Fatalf("%s %s: content type %q", method, path, ct)
	}
	if w.Body.Len() == 0 {
		return nil
	}
	var got map[string]any
	if err := json.NewDecoder(bytes.NewReader(w.Body.Bytes())).Decode(&got); err != nil {
		c.t.Fatal(err)
	}
	return got
}

func field(t *testing.T, object map[string]any, key string) string {
	t.Helper()
	v, ok := object[key].(string)
	if !ok {
		t.Fatalf("%s missing or not string in %#v", key, object)
	}
	return v
}

func items(t *testing.T, object map[string]any, key string) []any {
	t.Helper()
	v, ok := object[key].([]any)
	if !ok {
		t.Fatalf("%s missing or not array in %#v", key, object)
	}
	return v
}

func errorCode(t *testing.T, object map[string]any, want string) {
	t.Helper()
	err, ok := object["error"].(map[string]any)
	if !ok || err["code"] != want || err["message"] == "" {
		t.Fatalf("error %#v, want %s", object, want)
	}
}

func TestStateAndRestart(t *testing.T) {
	c := newClient(t)
	if len(items(t, c.call("GET", "/api/v1/campaigns", "", 200), "campaigns")) != 4 {
		t.Fatal("demo campaign seeds missing")
	}
	created := c.call("POST", "/api/v1/campaigns", `{"name":"新增活动"}`, 201)
	id := field(t, created, "id")
	if id == "" || field(t, c.call("GET", "/api/v1/campaigns/"+id, "", 200), "name") != "新增活动" {
		t.Fatal("created state not retained")
	}
	updated := c.call("PUT", "/api/v1/campaigns/"+id, `{"id":"`+id+`","name":"改名活动","treatments":[]}`, 200)
	if field(t, updated, "id") != id || field(t, updated, "name") != "改名活动" {
		t.Fatal("rename changed identity")
	}
	for _, seed := range []string{"campaign-branch", "campaign-single", "campaign-empty", "campaign-no-edges", id} {
		c.call("DELETE", "/api/v1/campaigns/"+seed, "", 204)
	}
	if len(items(t, c.call("GET", "/api/v1/campaigns", "", 200), "campaigns")) != 0 {
		t.Fatal("deleted campaigns reappeared")
	}
	if len(items(t, newClient(t).call("GET", "/api/v1/campaigns", "", 200), "campaigns")) != 4 {
		t.Fatal("new process state should begin from seeds")
	}
}

func TestTreatmentsHaveNoAudienceAndRemainAtomic(t *testing.T) {
	c := newClient(t)
	base := "/api/v1/campaigns/campaign-empty/treatments"
	c.call("PUT", "/api/v1/campaigns/campaign-empty/audience", `{"expectedAudience":null,"audience":{"type":"group","groupId":"group-new"}}`, 200)
	create := func(body string, want int) map[string]any {
		t.Helper()
		return c.call("POST", base, body, want)
	}
	for _, body := range []string{
		`{"name":"未知动作","actionId":"action-missing"}`,
		`{"name":"停用动作","actionId":"action-retired"}`,
		`{"name":"缺展示位","actionId":"action-show-image"}`,
		`{"name":"旧模型字段","actionId":"action-sms","audienceRef":{"type":"group","groupId":"group-new"}}`,
	} {
		want := 400
		if strings.Contains(body, "action-missing") || strings.Contains(body, "action-retired") || strings.Contains(body, "action-show-image") {
			want = 422
		}
		got := create(body, want)
		if got == nil {
			t.Fatal("invalid treatment request unexpectedly succeeded")
		}
	}
	created := create(`{"name":"欢迎短信","actionId":"action-sms"}`, 201)
	id := field(t, created, "id")
	if _, exists := created["audienceRef"]; exists {
		t.Fatal("treatment still exposes its own audience")
	}
	before := c.call("GET", "/api/v1/campaigns/campaign-empty", "", 200)
	c.call("PUT", base+"/"+id, `{"id":"`+id+`","name":"错误动作","actionId":"action-missing"}`, 422)
	after := c.call("GET", "/api/v1/campaigns/campaign-empty", "", 200)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed update polluted state")
	}
	c.call("DELETE", base+"/"+id, "", 204)
}

func TestCampaignFlowEntryAndReceiptRoutes(t *testing.T) {
	c := newClient(t)
	path := "/api/v1/campaigns/campaign-branch"
	branch := c.call("GET", path, "", 200)
	if _, ok := branch["audience"]; !ok {
		t.Fatal("activity audience missing")
	}
	flow := branch["activityFlow"].(map[string]any)
	entries := append([]any{}, items(t, flow, "entries")...)
	edges := append([]any{}, items(t, flow, "edges")...)
	if len(entries) != 1 || field(t, entries[0].(map[string]any), "groupId") != "group-recall" {
		t.Fatalf("expected a single campaign audience route, got %#v", entries)
	}
	if _, exists := items(t, branch, "treatments")[0].(map[string]any)["audienceRef"]; exists {
		t.Fatal("treatment has a duplicated audience reference")
	}
	errorCode(t, c.call("PUT", path+"/flow", `{"id":"flow-recall","entries":[],"edges":[]}`, 400), "expected_flow_required")
	errorCode(t, c.call("DELETE", path+"/treatments/treatment-rcs", "", 409), "entry_conflict")

	flow["expectedEntries"] = entries
	flow["expectedEdges"] = edges
	flow["entries"] = []any{map[string]any{"groupId": "group-new", "kind": "start", "treatmentIds": []any{"treatment-rcs"}}}
	body, err := json.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	errorCode(t, c.call("PUT", path+"/flow", string(body), 409), "entry_outside_audience")

	updatedEdges := append(edges, map[string]any{"id": "edge-email-sms", "fromTreatmentId": "treatment-email", "toTreatmentId": "treatment-sms", "conditionLabel": "成功"})
	flow["entries"] = entries
	flow["edges"] = updatedEdges
	body, err = json.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	updated := c.call("PUT", path+"/flow", string(body), 200)
	updatedFlow := updated["activityFlow"].(map[string]any)
	if len(items(t, updatedFlow, "edges")) != 3 {
		t.Fatal("valid activity flow update was not retained")
	}

	flow["expectedEdges"] = edges
	flow["edges"] = edges
	body, err = json.Marshal(flow)
	if err != nil {
		t.Fatal(err)
	}
	errorCode(t, c.call("PUT", path+"/flow", string(body), 409), "flow_conflict")
	if !reflect.DeepEqual(updated, c.call("GET", path, "", 200)) {
		t.Fatal("stale flow update changed current edges")
	}
	bad := map[string]any{"id": "flow-recall", "expectedEntries": entries, "expectedEdges": updatedEdges, "entries": entries, "edges": []any{map[string]any{"id": "bad", "fromTreatmentId": "treatment-rcs", "toTreatmentId": "treatment-email", "conditionLabel": "已读"}}}
	body, err = json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	errorCode(t, c.call("PUT", path+"/flow", string(body), 422), "invalid_edge_condition")
}

func TestSingleAudienceSelectionAndUsage(t *testing.T) {
	c := newClient(t)
	refs := items(t, c.call("GET", "/api/v1/audience-references", "", 200), "audienceReferences")
	if len(refs) != 7 {
		t.Fatalf("audience catalog size = %d", len(refs))
	}
	path := "/api/v1/campaigns/campaign-empty/audience"
	errorCode(t, c.call("PUT", path, `{"audience":{"type":"group","groupId":"group-new"}}`, 400), "expected_audience_required")
	selected := c.call("PUT", path, `{"expectedAudience":null,"audience":{"type":"group","groupId":"group-new"}}`, 200)
	selection := selected["audience"].(map[string]any)
	if field(t, selection, "groupId") != "group-new" || selection["type"] != "group" {
		t.Fatalf("activity audience selection = %#v", selection)
	}
	errorCode(t, c.call("PUT", path, `{"expectedAudience":null,"audience":{"type":"bundle","bundleId":"bundle-warmup"}}`, 409), "audience_conflict")
	errorCode(t, c.call("PUT", path, `{"expectedAudience":{"type":"group","groupId":"group-new"},"audience":{"type":"group","groupId":"group-unknown"}}`, 422), "unknown_audience")
	if !reflect.DeepEqual(selected, c.call("GET", "/api/v1/campaigns/campaign-empty", "", 200)) {
		t.Fatal("invalid audience selection changed state")
	}
	errorCode(t, c.call("PUT", "/api/v1/campaigns/campaign-empty/audiences", `{"audienceRefs":[]}`, 404), "not_found")
	usage := c.call("GET", "/api/v1/audience-reference-usages?type=group&groupId=group-new", "", 200)
	usages := items(t, usage, "usages")
	if len(usages) != 2 || field(t, usages[len(usages)-1].(map[string]any), "role") != "activity-audience" {
		t.Fatalf("activity audience usage = %#v", usages)
	}
	created := c.call("POST", "/api/v1/campaigns/campaign-empty/treatments", `{"name":"测试","actionId":"action-sms"}`, 201)
	if len(items(t, c.call("GET", "/api/v1/audience-reference-usages?type=group&groupId=group-new", "", 200), "usages")) != 2 {
		t.Fatal("creating a treatment changed audience usage")
	}
	id := field(t, created, "id")
	c.call("DELETE", "/api/v1/campaigns/campaign-empty/treatments/"+id, "", 204)
	c.call("PUT", path, `{"expectedAudience":{"type":"group","groupId":"group-new"},"audience":null}`, 200)
	errorCode(t, c.call("GET", "/api/v1/audience-reference-usages?type=arm&groupId=group-arm-a", "", 400), "invalid_query")
}

func TestContentTypeAndNoncanonicalPath(t *testing.T) {
	c := newClient(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(`{"name":"不应创建"}`))
	request.Header.Set("Content-Type", "application/jsonp")
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, request)
	if w.Code != 415 || !strings.Contains(w.Body.String(), `"unsupported_media_type"`) {
		t.Fatalf("invalid media type: %d %s", w.Code, w.Body.String())
	}
	if len(items(t, c.call("GET", "/api/v1/campaigns", "", 200), "campaigns")) != 4 {
		t.Fatal("invalid media type changed state")
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(`{"name":"合法 JSON"}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	w = httptest.NewRecorder()
	c.h.ServeHTTP(w, request)
	if w.Code != 201 {
		t.Fatalf("JSON with charset rejected: %d %s", w.Code, w.Body.String())
	}
	errorCode(t, c.call("GET", "/api/v1//campaigns", "", 404), "not_found")
}
