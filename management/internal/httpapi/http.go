package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"time"

	"github.com/snack8310/gmp/management/internal/management"
)

type api struct{ store *management.Store }

func NewHandler(store *management.Store, logger *slog.Logger) http.Handler {
	a := &api{store: store}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/campaigns", a.campaigns)
	mux.HandleFunc("/api/v1/campaigns/{campaignId}", a.campaign)
	mux.HandleFunc("/api/v1/campaigns/{campaignId}/treatments", a.treatments)
	mux.HandleFunc("/api/v1/campaigns/{campaignId}/treatments/{treatmentId}", a.treatment)
	mux.HandleFunc("/api/v1/actions", a.actions)
	mux.HandleFunc("/api/v1/audience-references", a.references)
	mux.HandleFunc("/api/v1/audience-reference-usages", a.usages)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, &management.Problem{Status: 404, Code: "not_found", Message: "接口不存在"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		tracked := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		if path.Clean(r.URL.Path) != r.URL.Path {
			writeProblem(tracked, &management.Problem{Status: 404, Code: "not_found", Message: "接口不存在"})
		} else {
			mux.ServeHTTP(tracked, r)
		}
		logger.Info("http_request", "method", r.Method, "status", tracked.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != http.StatusNoContent {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeProblem(w http.ResponseWriter, p *management.Problem) {
	writeJSON(w, p.Status, map[string]any{"error": map[string]string{"code": p.Code, "message": p.Message}})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeProblem(w, &management.Problem{Status: 405, Code: "method_not_allowed", Message: "请求方法不支持"})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeProblem(w, &management.Problem{Status: 415, Code: "unsupported_media_type", Message: "请求体必须是 JSON"})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		writeProblem(w, &management.Problem{Status: 400, Code: "invalid_json", Message: "JSON 字段或格式无效"})
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeProblem(w, &management.Problem{Status: 400, Code: "invalid_json", Message: "请求体只能包含一个 JSON 对象"})
		return false
	}
	return true
}

func (a *api) campaigns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]any{"campaigns": a.store.Campaigns()})
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &input) {
			return
		}
		created, p := a.store.CreateCampaign(input.Name)
		if p != nil {
			writeProblem(w, p)
			return
		}
		writeJSON(w, 201, created)
	default:
		methodNotAllowed(w)
	}
}

func (a *api) campaign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("campaignId")
	switch r.Method {
	case http.MethodGet:
		c, p := a.store.Campaign(id)
		if p != nil {
			writeProblem(w, p)
			return
		}
		writeJSON(w, 200, c)
	case http.MethodPut:
		var input management.Campaign
		if !decode(w, r, &input) {
			return
		}
		c, p := a.store.UpdateCampaign(id, input)
		if p != nil {
			writeProblem(w, p)
			return
		}
		writeJSON(w, 200, c)
	case http.MethodDelete:
		if p := a.store.DeleteCampaign(id); p != nil {
			writeProblem(w, p)
			return
		}
		writeJSON(w, 204, nil)
	default:
		methodNotAllowed(w)
	}
}

func (a *api) treatments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var input struct {
		Name        string                 `json:"name"`
		ActionID    string                 `json:"actionId"`
		AudienceRef management.AudienceRef `json:"audienceRef"`
		SlotID      string                 `json:"slotId"`
	}
	if !decode(w, r, &input) {
		return
	}
	t := management.Treatment{Name: input.Name, ActionID: input.ActionID, AudienceRef: input.AudienceRef, SlotID: input.SlotID}
	created, p := a.store.CreateTreatment(r.PathValue("campaignId"), t)
	if p != nil {
		writeProblem(w, p)
		return
	}
	writeJSON(w, 201, created)
}

func (a *api) treatment(w http.ResponseWriter, r *http.Request) {
	campaignID, treatmentID := r.PathValue("campaignId"), r.PathValue("treatmentId")
	switch r.Method {
	case http.MethodPut:
		var input management.Treatment
		if !decode(w, r, &input) {
			return
		}
		updated, p := a.store.UpdateTreatment(campaignID, treatmentID, input)
		if p != nil {
			writeProblem(w, p)
			return
		}
		writeJSON(w, 200, updated)
	case http.MethodDelete:
		if p := a.store.DeleteTreatment(campaignID, treatmentID); p != nil {
			writeProblem(w, p)
			return
		}
		writeJSON(w, 204, nil)
	default:
		methodNotAllowed(w)
	}
}

func (a *api) actions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, 200, map[string]any{"actions": a.store.Actions()})
}

func (a *api) references(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, 200, map[string]any{"audienceReferences": a.store.AudienceReferences()})
}

func (a *api) usages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := r.URL.Query()
	if query.Get("type") == "group" {
		if _, present := query["bundleId"]; present {
			writeProblem(w, &management.Problem{Status: 400, Code: "invalid_query", Message: "普通组查询不得带 bundleId"})
			return
		}
	}
	for key, values := range query {
		if (key != "type" && key != "groupId" && key != "bundleId") || len(values) != 1 {
			writeProblem(w, &management.Problem{Status: 400, Code: "invalid_query", Message: "查询参数无效"})
			return
		}
	}
	ref := management.AudienceRef{Type: query.Get("type"), GroupID: query.Get("groupId"), BundleID: query.Get("bundleId")}
	usages, p := a.store.Usages(ref)
	if p != nil {
		writeProblem(w, p)
		return
	}
	writeJSON(w, 200, map[string]any{"usages": usages})
}
