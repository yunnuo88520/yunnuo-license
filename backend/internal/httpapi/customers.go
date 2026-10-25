package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

type customerKey struct{}

func customerFrom(ctx context.Context) domain.Customer {
	c, _ := ctx.Value(customerKey{}).(domain.Customer)
	return c
}
func (api *API) withCustomer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := api.service.AuthenticateCustomer(r.Context(), bearerToken(r))
		if err != nil {
			writeError(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), customerKey{}, c)))
	}
}
func (api *API) registerCustomerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/customers", api.withAdmin(api.customers, adminReadRoles...))
	mux.HandleFunc("POST /admin/customers", api.withAdmin(api.createCustomer, adminWriteRoles...))
	mux.HandleFunc("POST /admin/customers/{userID}/{action}", api.withAdmin(api.customerStatus, adminWriteRoles...))
	mux.HandleFunc("GET /admin/products/{productID}/binding-policy", api.withAdmin(api.bindingPolicy, adminReadRoles...))
	mux.HandleFunc("POST /admin/products/{productID}/binding-policy", api.withAdmin(api.saveBindingPolicy, adminWriteRoles...))
	mux.HandleFunc("POST /admin/licenses/{licenseNo}/owner", api.withAdmin(api.assignOwner, adminWriteRoles...))
	mux.HandleFunc("GET /admin/operation-logs", api.withAdmin(api.operationLogs, adminReadRoles...))
	mux.HandleFunc("GET /admin/operation-logs/export", api.withAdmin(api.exportOperations, adminReadRoles...))
	mux.HandleFunc("POST /user/login", withRateLimit(api.adminLoginLimit, api.customerLogin))
	mux.HandleFunc("GET /user/profile", api.withCustomer(func(w http.ResponseWriter, r *http.Request) { writeOK(w, r, customerFrom(r.Context())) }))
	mux.HandleFunc("POST /user/password", api.withCustomer(api.customerPassword))
	mux.HandleFunc("GET /user/licenses", api.withCustomer(api.customerLicenses))
	mux.HandleFunc("POST /user/activate", api.withCustomer(api.customerActivate))
	mux.HandleFunc("GET /user/licenses/{licenseNo}", api.withCustomer(api.customerLicense))
	mux.HandleFunc("POST /user/licenses/{licenseNo}/renew", api.withCustomer(api.customerRenew))
	mux.HandleFunc("POST /user/licenses/{licenseNo}/rebind", withRateLimit(api.customerRebindLimit, api.withCustomer(api.customerRebind)))
	mux.HandleFunc("GET /user-console", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/user-console/", 307) })
	mux.HandleFunc("GET /user-console/", func(w http.ResponseWriter, r *http.Request) {
		serveStaticApp(w, r, api.customerStaticDir, strings.TrimPrefix(r.URL.Path, "/user-console"))
	})
}
func (api *API) customers(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.Customers(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) createCustomer(w http.ResponseWriter, r *http.Request) {
	var input service.CustomerInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.CreateCustomer(r.Context(), input, adminSessionFrom(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) customerStatus(w http.ResponseWriter, r *http.Request) {
	status, ok := map[string]string{"enable": "active", "disable": "disabled"}[r.PathValue("action")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := api.service.SetCustomerStatus(r.Context(), r.PathValue("userID"), status, adminSessionFrom(r.Context()).UserID); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]string{"status": status})
}
func (api *API) customerLogin(w http.ResponseWriter, r *http.Request) {
	var input service.AdminLoginInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.CustomerLogin(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if a := service.ActorFrom(r.Context()); a != nil {
		a.Type = "customer"
		a.ID = result.User.ID
		a.Role = "customer"
	}
	writeOK(w, r, result)
}
func (api *API) customerPassword(w http.ResponseWriter, r *http.Request) {
	var input service.ChangeAdminPasswordInput
	if !decode(w, r, &input) {
		return
	}
	if err := api.service.CustomerPassword(r.Context(), customerFrom(r.Context()).ID, input); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"changed": true})
}
func (api *API) customerLicenses(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.CustomerLicenses(r.Context(), customerFrom(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) customerLicense(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.CustomerLicense(r.Context(), customerFrom(r.Context()).ID, r.PathValue("licenseNo"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) customerActivate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ApplicationID   string `json:"application_id"`
		ApplicationCode string `json:"application_code"`
		CardCode        string `json:"card_code"`
		BindValue       string `json:"bind_value"`
	}
	if !decode(w, r, &input) {
		return
	}
	application := strings.TrimSpace(input.ApplicationID)
	if application != "" && strings.TrimSpace(input.ApplicationCode) != "" {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	if application == "" {
		application = input.ApplicationCode
	}
	result, err := api.service.CustomerActivate(r.Context(), customerFrom(r.Context()).ID, application, service.ActivateInput{CardCode: input.CardCode, BindValue: input.BindValue, ClientIP: api.clientIP(r), UserAgent: r.UserAgent()})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) customerRenew(w http.ResponseWriter, r *http.Request) {
	var input struct {
		BindingID string `json:"binding_id"`
		CardCode  string `json:"card_code"`
	}
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.CustomerRenew(r.Context(), customerFrom(r.Context()).ID, r.PathValue("licenseNo"), input.BindingID, input.CardCode)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) customerRebind(w http.ResponseWriter, r *http.Request) {
	var input service.RebindInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.CustomerRebind(r.Context(), customerFrom(r.Context()).ID, r.PathValue("licenseNo"), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) bindingPolicy(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.BindingPolicy(r.Context(), r.PathValue("productID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) saveBindingPolicy(w http.ResponseWriter, r *http.Request) {
	var input domain.BindingPolicy
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.SetBindingPolicy(r.Context(), r.PathValue("productID"), adminSessionFrom(r.Context()).UserID, input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
func (api *API) assignOwner(w http.ResponseWriter, r *http.Request) {
	var input service.AssignOwnerInput
	if !decode(w, r, &input) {
		return
	}
	if err := api.service.AssignOwner(r.Context(), r.PathValue("licenseNo"), adminSessionFrom(r.Context()).UserID, input); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"assigned": true})
}
