package httpapi

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/license-block.html
var blockPages embed.FS
var licenseBlockTemplate = template.Must(template.ParseFS(blockPages, "templates/license-block.html"))

func (api *API) v2BlockPage(w http.ResponseWriter, r *http.Request) {
	data, e := api.service.Licensing().PublicBlockPolicy(r.Context(), r.PathValue("appID"))
	if e != nil {
		writeError(w, r, e)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	_ = licenseBlockTemplate.Execute(w, data)
}
