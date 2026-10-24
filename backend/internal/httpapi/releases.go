package httpapi

import (
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/domain"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func (api *API) registerReleaseRoutes(mux *http.ServeMux) {
	for _, prefix := range []string{"/admin", "/developer"} {
		readRoles, writeRoles := adminReadRoles, adminWriteRoles
		if prefix == "/developer" {
			readRoles = []string{domain.AdminRoleDeveloper}
			writeRoles = readRoles
		}
		wrap := func(handler http.HandlerFunc, roles []string) http.HandlerFunc {
			if prefix == "/developer" {
				handler = api.withDeveloperProduct(handler)
			}
			return api.withAdmin(handler, roles...)
		}
		mux.HandleFunc("GET "+prefix+"/products/{productID}/updates", wrap(api.productUpdates, readRoles))
		mux.HandleFunc("POST "+prefix+"/products/{productID}/updates", wrap(api.configureProductUpdates, writeRoles))
		mux.HandleFunc("GET "+prefix+"/products/{productID}/releases", wrap(api.listReleases, readRoles))
		mux.HandleFunc("POST "+prefix+"/products/{productID}/releases", wrap(api.uploadRelease, writeRoles))
		mux.HandleFunc("POST "+prefix+"/products/{productID}/releases/{releaseID}/{action}", wrap(api.changeReleaseStatus, writeRoles))
	}
	mux.HandleFunc("POST /v1/updates/check", withRateLimit(api.publicQueryLimit, api.checkUpdate))
	mux.HandleFunc("POST /v1/updates/{releaseID}/download", withRateLimit(api.publicQueryLimit, api.downloadRelease))
	mux.HandleFunc("GET /v1/updates/{releaseID}/download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "POST")
		http.Error(w, "use authenticated POST to download", http.StatusMethodNotAllowed)
	})
}

func (api *API) productUpdates(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.ProductUpdates(r.Context(), r.PathValue("productID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) configureProductUpdates(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.SetProductUpdates(r.Context(), r.PathValue("productID"), input.Enabled, adminSessionFrom(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) listReleases(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.ListReleases(r.Context(), r.PathValue("productID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) uploadRelease(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxReleaseSize+(1<<20))
	defer r.Body.Close()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("package")
	if err != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	defer file.Close()
	result, err := api.service.UploadRelease(r.Context(), service.UploadReleaseInput{ProductID: r.PathValue("productID"), Version: r.FormValue("version"), Notes: r.FormValue("notes"), Filename: header.Filename}, file, adminSessionFrom(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) changeReleaseStatus(w http.ResponseWriter, r *http.Request) {
	statuses := map[string]string{"publish": "published", "withdraw": "withdrawn"}
	status, ok := statuses[r.PathValue("action")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	result, err := api.service.ChangeReleaseStatus(r.Context(), r.PathValue("productID"), r.PathValue("releaseID"), status, adminSessionFrom(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) checkUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input service.CheckUpdateInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP, input.UserAgent = api.clientIP(r), r.UserAgent()
	result, err := api.service.CheckUpdate(r.Context(), input)
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) downloadRelease(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input service.VerifyInput
	if !decode(w, r, &input) {
		return
	}
	input.ClientIP, input.UserAgent = api.clientIP(r), r.UserAgent()
	release, file, err := api.service.DownloadRelease(r.Context(), r.PathValue("releaseID"), input)
	w.Header().Set("Cache-Control", "private, no-store")
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": release.Filename}))
	w.Header().Set("Content-Length", strconv.FormatInt(release.Size, 10))
	_, _ = io.Copy(w, file)
}

func (api *API) withDeveloperProduct(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := api.service.RequireDeveloperProduct(r.Context(), r.PathValue("productID"), adminSessionFrom(r.Context()).UserID); err != nil {
			writeError(w, r, err)
			return
		}
		next(w, r)
	}
}

func (api *API) registerDeveloperRoutes(mux *http.ServeMux) {
	wrap := func(h http.HandlerFunc) http.HandlerFunc { return api.withAdmin(h, domain.AdminRoleDeveloper) }
	mux.HandleFunc("POST /developer/login", withRateLimit(api.adminLoginLimit, api.developerLogin))
	mux.HandleFunc("GET /developer/profile", wrap(api.adminProfile))
	mux.HandleFunc("POST /developer/password", wrap(api.changeAdminPassword))
	mux.HandleFunc("GET /developer/products", wrap(api.developerProducts))
	mux.HandleFunc("POST /developer/products", wrap(api.developerCreateProduct))
	mux.HandleFunc("POST /developer/products/{productID}/card-batches", wrap(api.withDeveloperProduct(api.developerCreateCards)))
	mux.HandleFunc("GET /developer/products/{productID}/licenses", wrap(api.withDeveloperProduct(api.developerLicenses)))
	mux.HandleFunc("POST /admin/products/{productID}/developer", api.withAdmin(api.assignDeveloper, adminWriteRoles...))
	mux.HandleFunc("POST /admin/developers/{userID}/{action}", api.withAdmin(api.developerStatus, adminSuperRoles...))
	mux.HandleFunc("GET /developer-console", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/developer-console/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("GET /developer-console/", func(w http.ResponseWriter, r *http.Request) {
		serveStaticApp(w, r, api.developerStaticDir, strings.TrimPrefix(r.URL.Path, "/developer-console"))
	})
}

func (api *API) developerLogin(w http.ResponseWriter, r *http.Request) {
	var input service.AdminLoginInput
	if !decode(w, r, &input) {
		return
	}
	result, err := api.service.AdminLogin(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if result.User.Role != domain.AdminRoleDeveloper {
		writeError(w, r, service.ErrPermissionDenied)
		return
	}
	writeOK(w, r, result)
}

func (api *API) developerProducts(w http.ResponseWriter, r *http.Request) {
	result, err := api.service.ListDeveloperProducts(r.Context(), adminSessionFrom(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) developerCreateProduct(w http.ResponseWriter, r *http.Request) {
	var input service.CreateProductInput
	if !decode(w, r, &input) {
		return
	}
	input.DeveloperID = adminSessionFrom(r.Context()).UserID
	result, err := api.service.CreateProduct(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) assignDeveloper(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DeveloperID string `json:"developer_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	err := api.service.AssignDeveloper(r.Context(), r.PathValue("productID"), input.DeveloperID, adminSessionFrom(r.Context()).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]bool{"assigned": true})
}

func (api *API) developerStatus(w http.ResponseWriter, r *http.Request) {
	status, ok := map[string]string{"enable": domain.AdminUserActive, "disable": domain.AdminUserDisabled}[r.PathValue("action")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := api.service.SetDeveloperStatus(r.Context(), r.PathValue("userID"), status, adminSessionFrom(r.Context()).UserID); err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, map[string]string{"status": status})
}

func (api *API) developerCreateCards(w http.ResponseWriter, r *http.Request) {
	var input service.CreateCardBatchInput
	if !decode(w, r, &input) {
		return
	}
	input.ProductID = r.PathValue("productID")
	result, err := api.service.CreateCardBatch(r.Context(), input)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}

func (api *API) developerLicenses(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	result, err := api.service.ListLicensesPage(r.Context(), service.LicenseListFilter{ProductID: r.PathValue("productID"), Page: page, PageSize: 20})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeOK(w, r, result)
}
