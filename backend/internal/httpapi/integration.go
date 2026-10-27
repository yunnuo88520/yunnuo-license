package httpapi

import (
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/yunnuo88520/yunnuo-license/backend/internal/integration"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
)

func (api *API) integrationKit(w http.ResponseWriter, r *http.Request) {
	p, err := api.service.IntegrationProduct(r.Context(), r.PathValue("productID"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	keys, err := api.service.ProductKeys(r.Context(), p.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if p.AuthorizationTerms != nil {
		p = p.AuthorizationTerms.Apply(p)
	}
	config := integration.Config{ProductID: p.ID, BaseURL: r.URL.Query().Get("base_url"), AppKey: p.ID, ProductCode: p.Code, BindMode: p.BindMode, BindValue: strings.TrimSpace(r.URL.Query().Get("bind_value")), PublicKeys: map[int]string{}}
	for _, key := range keys.Keys {
		config.PublicKeys[key.KeyVersion] = key.PublicKeyPEM
	}
	language := r.URL.Query().Get("language")
	if err = integration.Validate(config, language); err != nil {
		writeError(w, r, service.ErrInvalidRequest)
		return
	}
	content, err := integration.Build(envOr("YN_SDK_DIR", "../sdk"), language, config)
	if err != nil {
		writeError(w, r, service.AppError{Code: "INTEGRATION_ASSETS_UNAVAILABLE", Message: "SDK integration assets are unavailable; check YN_SDK_DIR", HTTPStatus: 503})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.ID + "-" + language + "-integration.zip"}))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(content)
}
