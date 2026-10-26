package httpapi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func operationCategory(route string) string {
	if strings.Contains(route, "/credentials") || strings.Contains(route, "/v1/app/") {
		return "applications"
	}
	for _, v := range []struct{ match, category string }{{"recovery", "account"}, {"security", "security"}, {"balance", "finance"}, {"money-ledgers", "finance"}, {"login", "account"}, {"password", "account"}, {"owner", "ownership"}, {"binding", "binding"}, {"rebind", "binding"}, {"unbind", "binding"}, {"renew", "renewal"}, {"releases", "updates"}, {"updates", "updates"}, {"risk", "security"}, {"packages", "packages"}, {"group", "applications"}, {"agent", "agents"}, {"card", "cards"}, {"license", "licenses"}, {"product", "applications"}, {"customer", "users"}, {"user", "users"}, {"audit", "audit"}, {"operation-logs", "audit"}} {
		if strings.Contains(route, v.match) {
			return v.category
		}
	}
	return "platform"
}

type operationWriter struct {
	http.ResponseWriter
	status    int
	errorBody []byte
}

func (w *operationWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *operationWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	if w.status >= 400 && len(w.errorBody) < 4096 {
		left := 4096 - len(w.errorBody)
		w.errorBody = append(w.errorBody, b[:min(len(b), left)]...)
	}
	return w.ResponseWriter.Write(b)
}
func (api *API) operationAudit(next http.Handler, mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, route := mux.Handler(r)
		business := false
		for _, prefix := range []string{"/admin/", "/agent/", "/developer/", "/user/", "/v1/"} {
			if strings.HasPrefix(r.URL.Path, prefix) {
				business = true
			}
		}
		if !business {
			next.ServeHTTP(w, r)
			return
		}
		if route == "" || route == "GET /" {
			route = "unmatched API"
		}
		a := &service.AuditActor{Type: "anonymous", RequestID: requestIDFrom(r), ClientIP: api.clientIP(r)}
		if token := bearerToken(r); token != "" {
			if admin, err := api.service.AuthenticateAdminToken(r.Context(), token); err == nil {
				a.Type = "admin"
				a.ID = admin.UserID
				a.Role = admin.Role
				if admin.Role == "developer" {
					a.Type = "developer"
				}
			} else if customer, err := api.service.AuthenticateCustomer(r.Context(), token); err == nil {
				a.Type = "customer"
				a.ID = customer.ID
				a.Role = "customer"
			} else if agent, err := api.service.AuthenticateAgentToken(r.Context(), token); err == nil {
				a.Type = "agent"
				a.ID = agent.UserID
				a.Role = agent.Role
			}
		}
		ctx := service.WithAuditActor(r.Context(), a)
		ticket, err := api.service.BeginOperation(ctx, requestIDFrom(r), operationCategory(route), r.Method, route, api.clientIP(r), a)
		if err != nil {
			writeError(w, r, service.AppError{Code: "AUDIT_UNAVAILABLE", Message: "operation logging is unavailable; retry later", HTTPStatus: 503})
			return
		}
		capture := &operationWriter{ResponseWriter: w}
		audited := r.WithContext(ctx)
		next.ServeHTTP(capture, audited)
		for _, key := range []string{"licenseNo", "productID", "userID", "agentID", "packageID", "tierID", "caseID"} {
			if id := audited.PathValue(key); id != "" {
				ticket.Log.ObjectID = id[:min(len(id), 96)]
				break
			}
		}
		status := capture.status
		if status == 0 {
			status = 200
		}
		code := ""
		var body struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(capture.errorBody, &body) == nil {
			code = body.Error.Code
		}
		finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := api.service.FinishOperation(finish, ticket, a, status, code); err != nil {
			log.Printf("operation audit remains pending request_id=%s", requestIDFrom(r))
		}
	})
}
func operationFilter(r *http.Request) service.OperationFilter {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))
	return service.OperationFilter{Category: q.Get("category"), Actor: q.Get("actor"), Result: q.Get("result"), Query: q.Get("q"), From: q.Get("from"), To: q.Get("to"), Page: page, PageSize: size}
}
func (api *API) operationLogs(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.OperationLogs(r.Context(), operationFilter(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) exportOperations(w http.ResponseWriter, r *http.Request) {
	filter := operationFilter(r)
	filter.Page = 1
	filter.PageSize = 5000
	result, err := api.service.OperationLogs(r.Context(), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="operation-logs.csv"`)
	writer := csv.NewWriter(w)
	defer writer.Flush()
	writer.Write([]string{"时间", "分类", "操作者类型", "操作者ID", "动作", "路由", "结果", "状态码", "错误码", "请求ID"})
	for _, l := range result.Items {
		writer.Write([]string{l.CreatedAt.Format(time.RFC3339), l.Category, l.ActorType, l.ActorID, l.Method, l.Route, l.Result, strconv.Itoa(l.HTTPStatus), l.ErrorCode, l.RequestID})
	}
}
