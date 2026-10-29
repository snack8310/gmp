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

func TestTreatmentValidationAndAtomicity(t *testing.T) {
	c := newClient(t)
	base := "/api/v1/campaigns/campaign-empty/treatments"
	create := func(name, action, audience, slot string, want int) map[string]any {
		t.Helper()
		return c.call("POST", base, `{"name":"`+name+`","actionId":"`+action+`","audienceRef":`+audience+`,"slotId":"`+slot+`"}`, want)
	}
	group := `{"type":"group","groupId":"group-new"}`
	armA := `{"type":"arm","bundleId":"bundle-warmup","groupId":"group-arm-a"}`
	armB := `{"type":"arm","bundleId":"bundle-warmup","groupId":"group-arm-b"}`
	for _, tc := range []struct{ action, audience, slot, code string }{
		{"action-missing", group, "", "unknown_action"},
		{"action-retired", group, "", "unavailable_action"},
		{"action-sms", `{"type":"group","groupId":"group-missing"}`, "", "unknown_audience"},
		{"action-sms", `{"type":"group","groupId":"group-removed"}`, "", "unavailable_audience"},
		{"action-sms", `{"type":"arm","bundleId":"bundle-wrong","groupId":"group-arm-a"}`, "", "unknown_audience"},
		{"action-sms", `{"type":"arm","bundleId":"bundle-retired","groupId":"group-arm-retired"}`, "", "unavailable_audience"},
		{"action-sms", group, "slot-home-banner", "unexpected_slot"},
		{"action-show-image", group, "", "missing_slot"},
		{"action-show-image", group, "slot-wrong", "unknown_slot"},
	} {
		errorCode(t, create("无效", tc.action, tc.audience, tc.slot, 422), tc.code)
	}
	if len(items(t, c.call("GET", "/api/v1/campaigns/campaign-empty", "", 200), "treatments")) != 0 {
		t.Fatal("failed create polluted state")
	}
	a := create("图片 A", "action-show-image", armA, "slot-home-banner", 201)
	b := create("图片 B", "action-show-image", armB, "slot-home-banner", 201)
	if field(t, create("首页大图", "action-show-image", group, "slot-home-hero", 201), "slotId") != "slot-home-hero" {
		t.Fatal("second configured slot was not saved")
	}
	if field(t, a, "id") == field(t, b, "id") {
		t.Fatal("treatment IDs not unique")
	}
	errorCode(t, create("普通组图片", "action-show-image", group, "slot-home-banner", 409), "slot_conflict")
	push := create("短信", "action-sms", group, "", 201)
	pushID := field(t, push, "id")
	c.call("PUT", base+"/"+pushID, `{"id":"`+pushID+`","name":"改引短信","actionId":"action-sms","audienceRef":`+armA+`,"slotId":""}`, 200)
	before := c.call("GET", "/api/v1/campaigns/campaign-empty", "", 200)
	errorCode(t, c.call("PUT", base+"/"+pushID, `{"id":"`+pushID+`","name":"坏更新","actionId":"action-show-image","audienceRef":`+group+`,"slotId":"slot-home-banner"}`, 409), "slot_conflict")
	after := c.call("GET", "/api/v1/campaigns/campaign-empty", "", 200)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed update polluted state")
	}
	c.call("DELETE", base+"/"+pushID, "", 204)
}

func TestFlowAndWholeCampaignValidation(t *testing.T) {
	c := newClient(t)
	path := "/api/v1/campaigns/campaign-branch"
	branch := c.call("GET", path, "", 200)
	if _, ok := branch["activityFlow"]; !ok {
		t.Fatal("seed flow missing")
	}
	errorCode(t, c.call("DELETE", path+"/treatments/treatment-rcs", "", 409), "edge_conflict")
	if len(items(t, c.call("GET", path, "", 200), "treatments")) != 3 {
		t.Fatal("failed delete changed state")
	}
	bad := `{"id":"campaign-branch","name":"坏更新","treatments":[{"id":"treatment-rcs","name":"RCS","actionId":"action-rcs","audienceRef":{"type":"group","groupId":"group-recall"}}],"activityFlow":{"id":"flow-recall","edges":[{"id":"edge-success","fromTreatmentId":"treatment-rcs","toTreatmentId":"treatment-email","conditionLabel":"成功"}]}}`
	errorCode(t, c.call("PUT", path, bad, 409), "flow_read_only")
	if !reflect.DeepEqual(branch, c.call("GET", path, "", 200)) {
		t.Fatal("failed whole campaign update changed state")
	}
	noEdgesPath := "/api/v1/campaigns/campaign-no-edges"
	noEdges := c.call("GET", noEdgesPath, "", 200)
	treatments := items(t, noEdges, "treatments")
	noEdges["treatments"] = []any{treatments[0], treatments[0]}
	duplicate, err := json.Marshal(noEdges)
	if err != nil {
		t.Fatal(err)
	}
	errorCode(t, c.call("PUT", noEdgesPath, string(duplicate), 409), "duplicate_treatment")
	if len(items(t, c.call("GET", noEdgesPath, "", 200), "treatments")) != 2 {
		t.Fatal("duplicate treatment changed state")
	}
	other := c.call("GET", "/api/v1/campaigns/campaign-single", "", 200)
	otherTreatment := items(t, other, "treatments")[0]
	otherBody, err := json.Marshal(otherTreatment)
	if err != nil {
		t.Fatal(err)
	}
	errorCode(t, c.call("PUT", "/api/v1/campaigns/campaign-empty/treatments/treatment-welcome", string(otherBody), 404), "not_found")
}

func TestCatalogUsagesAndBadRequests(t *testing.T) {
	c := newClient(t)
	actions := items(t, c.call("GET", "/api/v1/actions", "", 200), "actions")
	actionsByID := map[string]map[string]any{}
	for _, item := range actions {
		action := item.(map[string]any)
		actionsByID[field(t, action, "id")] = action
	}
	for id, want := range map[string]struct{ requiresSlot, available bool }{
		"action-rcs":        {false, true},
		"action-email":      {false, true},
		"action-sms":        {false, true},
		"action-show-image": {true, true},
		"action-retired":    {false, false},
	} {
		got, exists := actionsByID[id]
		if !exists || got["requiresSlot"] != want.requiresSlot || got["available"] != want.available || field(t, got, "name") == "" {
			t.Fatalf("action %s: got %#v, want %#v", id, got, want)
		}
	}
	refs := items(t, c.call("GET", "/api/v1/audience-references", "", 200), "audienceReferences")
	found := map[string]bool{}
	for _, item := range refs {
		ref := item.(map[string]any)
		key := field(t, ref, "type") + "/" + field(t, ref, "groupId")
		if field(t, ref, "name") == "" {
			t.Fatalf("audience name missing: %#v", ref)
		}
		if bundle, ok := ref["bundleId"].(string); ok {
			key += "/" + bundle
			if field(t, ref, "bundleName") == "" {
				t.Fatalf("bundle name missing: %#v", ref)
			}
		}
		available, ok := ref["available"].(bool)
		if !ok {
			t.Fatalf("audience availability missing or invalid: %#v", ref)
		}
		found[key] = available
	}
	for key, want := range map[string]bool{
		"group/group-new":                      true,
		"group/group-recall":                   true,
		"group/group-removed":                  false,
		"arm/group-arm-a/bundle-warmup":        true,
		"arm/group-arm-b/bundle-warmup":        true,
		"arm/group-arm-control/bundle-warmup":  true,
		"arm/group-arm-retired/bundle-retired": false,
	} {
		got, exists := found[key]
		if !exists || got != want {
			t.Fatalf("audience identity or availability %s: got %t, exists %t, want %t", key, got, exists, want)
		}
	}
	usage := "/api/v1/audience-reference-usages?type=group&groupId=group-new"
	if len(items(t, c.call("GET", usage, "", 200), "usages")) != 2 {
		t.Fatal("seed usages missing")
	}
	created := c.call("POST", "/api/v1/campaigns/campaign-empty/treatments", `{"name":"测试","actionId":"action-sms","audienceRef":{"type":"group","groupId":"group-new"}}`, 201)
	if len(items(t, c.call("GET", usage, "", 200), "usages")) != 3 {
		t.Fatal("new usage missing")
	}
	id := field(t, created, "id")
	c.call("PUT", "/api/v1/campaigns/campaign-empty/treatments/"+id, `{"id":"`+id+`","name":"改引","actionId":"action-sms","audienceRef":{"type":"arm","bundleId":"bundle-warmup","groupId":"group-arm-a"}}`, 200)
	if len(items(t, c.call("GET", usage, "", 200), "usages")) != 2 {
		t.Fatal("old usage not removed")
	}
	armUsage := "/api/v1/audience-reference-usages?type=arm&bundleId=bundle-warmup&groupId=group-arm-a"
	if len(items(t, c.call("GET", armUsage, "", 200), "usages")) != 2 {
		t.Fatal("new arm usage missing")
	}
	c.call("DELETE", "/api/v1/campaigns/campaign-empty/treatments/"+id, "", 204)
	if len(items(t, c.call("GET", armUsage, "", 200), "usages")) != 1 {
		t.Fatal("deleted usage retained")
	}
	single := c.call("GET", "/api/v1/campaigns/campaign-single", "", 200)
	single["name"] = "改名欢迎"
	singleTreatments := items(t, single, "treatments")
	singleTreatments[0].(map[string]any)["audienceRef"] = map[string]any{"type": "group", "groupId": "group-recall"}
	changed, err := json.Marshal(single)
	if err != nil {
		t.Fatal(err)
	}
	c.call("PUT", "/api/v1/campaigns/campaign-single", string(changed), 200)
	if len(items(t, c.call("GET", usage, "", 200), "usages")) != 1 {
		t.Fatal("campaign replacement retained old usage")
	}
	recallUsage := "/api/v1/audience-reference-usages?type=group&groupId=group-recall"
	if len(items(t, c.call("GET", recallUsage, "", 200), "usages")) != 3 {
		t.Fatal("campaign replacement missing new usage")
	}
	c.call("DELETE", "/api/v1/campaigns/campaign-single", "", 204)
	if len(items(t, c.call("GET", recallUsage, "", 200), "usages")) != 2 {
		t.Fatal("campaign deletion retained usage")
	}
	errorCode(t, c.call("GET", "/api/v1/audience-reference-usages?type=arm&groupId=group-arm-a", "", 400), "invalid_query")
	errorCode(t, c.call("GET", "/api/v1/audience-reference-usages?type=group&groupId=group-new&bundleId=", "", 400), "invalid_query")
	errorCode(t, c.call("GET", "/api/v1/campaigns/missing", "", 404), "not_found")
	errorCode(t, c.call("POST", "/api/v1/campaigns", `{"id":"chosen","name":"bad"}`, 400), "invalid_json")
	errorCode(t, c.call("POST", "/api/v1/campaigns/campaign-empty/treatments", `{"name":"multi","actions":["action-sms","action-email"],"audienceRef":{"type":"group","groupId":"group-new"}}`, 400), "invalid_json")
	errorCode(t, c.call("POST", "/api/v1/campaigns/campaign-empty/treatments", `{"name":"缺动作","audienceRef":{"type":"group","groupId":"group-new"}}`, 400), "missing_field")
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
	pathResponse := c.call("GET", "/api/v1//campaigns", "", 404)
	errorCode(t, pathResponse, "not_found")
}
