package httpapi

import (
	"encoding/csv"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func historyFilter(r *http.Request) domain.HistoryFilter {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))
	return domain.HistoryFilter{Query: q.Get("q"), Status: q.Get("status"), Kind: q.Get("kind"), AgentID: q.Get("agent_id"), ProductID: q.Get("product_id"), From: q.Get("from"), To: q.Get("to"), Page: page, PageSize: size}
}
func csvText(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if value != "" && (strings.ContainsAny(value[:1], "\t\r\n") || trimmed != "" && strings.ContainsAny(trimmed[:1], "=+-@")) {
		return "'" + value
	}
	return value
}
func historyCSVRow(item any) []string {
	switch r := item.(type) {
	case domain.MoneyLedger:
		return []string{r.ID, r.CreatedAt.Format(time.RFC3339Nano), r.AgentID, r.Kind, strconv.FormatInt(r.Amount, 10), strconv.FormatInt(r.BalanceAfter, 10), r.PackageID, r.BatchID, strconv.FormatInt(r.UnitCost, 10), strconv.Itoa(r.Quantity), csvText(r.Remark), r.ActorID, r.RequestID}
	case domain.SecurityCase:
		return []string{r.ID, r.ProductID, r.LicenseNo, r.Signal, csvText(r.Subject), r.Status, r.Action, strconv.Itoa(r.Occurrences), r.FirstSeen.Format(time.RFC3339Nano), r.LastSeen.Format(time.RFC3339Nano), csvText(r.Evidence), csvText(r.ReviewReason), r.Reviewer}
	case domain.SecurityAppeal:
		return []string{r.ID, r.CaseID, r.UserID, r.Status, r.CreatedAt.Format(time.RFC3339Nano), csvText(r.Reason)}
	}
	return nil
}
func (api *API) history(w http.ResponseWriter, r *http.Request, scope, subject string, export bool) {
	kind := r.PathValue("kind")
	f := historyFilter(r)
	if !export {
		v, e := api.service.History(r.Context(), kind, scope, subject, f)
		commerceResponse(w, r, v, e)
		return
	}
	file, err := os.CreateTemp("", "yunnuo-history-*.csv")
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	writer := csv.NewWriter(file)
	headers := map[string][]string{"money": {"记录ID", "时间", "代理ID", "业务类型", "变动金额（分）", "结余（分）", "套餐ID", "批次ID", "单价（分）", "数量", "说明", "操作账号", "幂等请求号"}, "cases": {"处置编号", "应用ID", "授权号", "线索类型", "对象", "状态", "动作", "次数", "首次时间", "最近时间", "证据", "核实原因", "核实账号"}, "appeals": {"申诉ID", "处置编号", "用户ID", "状态", "时间", "申诉说明"}}
	writer.Write(headers[kind])
	err = api.service.StreamHistory(r.Context(), kind, scope, subject, f, func(item any) error { return writer.Write(historyCSVRow(item)) })
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	if _, err = file.Seek(0, 0); err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+kind+`-history.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	io.Copy(w, file)
}
func (api *API) registerHistoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/history/{kind}", api.withAdmin(func(w http.ResponseWriter, r *http.Request) { api.history(w, r, "admin", "", false) }, adminReadRoles...))
	mux.HandleFunc("GET /admin/history/{kind}/export", api.withAdmin(func(w http.ResponseWriter, r *http.Request) { api.history(w, r, "admin", "", true) }, adminReadRoles...))
	for _, export := range []bool{false, true} {
		path := "GET /agent/history/{kind}"
		if export {
			path += "/export"
		}
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			a, e := api.agentSession(r)
			if e != nil {
				writeError(w, r, e)
				return
			}
			api.history(w, r, "agent", a.AgentID, export)
		})
	}
	mux.HandleFunc("GET /user/history/{kind}", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		api.history(w, r, "customer", customerFrom(r.Context()).ID, false)
	}))
	mux.HandleFunc("GET /user/history/{kind}/export", api.withCustomer(func(w http.ResponseWriter, r *http.Request) {
		api.history(w, r, "customer", customerFrom(r.Context()).ID, true)
	}))
}
