package httpapi

import (
	"context"
	"github.com/yunnuo88520/yunnuo-license/backend/internal/service"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const clientIPContext contextKey = "verified_client_ip"

func containsIP(networks []netip.Prefix, ip netip.Addr) bool {
	for _, network := range networks {
		if network.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().String()
	}
	return host
}
func (api *API) resolveClientIP(r *http.Request) string {
	peer := peerIP(r)
	addr, err := netip.ParseAddr(peer)
	if err != nil || !api.trustProxyHeaders || !containsIP(api.trustedProxyNetworks, addr) {
		return peer
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 32 {
			return peer
		}
		chain := make([]netip.Addr, len(parts))
		for i, part := range parts {
			ip, e := netip.ParseAddr(strings.TrimSpace(part))
			if e != nil {
				return peer
			}
			chain[i] = ip.Unmap()
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !containsIP(api.trustedProxyNetworks, chain[i]) || i == 0 {
				return chain[i].String()
			}
		}
	}
	if ip, e := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); e == nil {
		return ip.Unmap().String()
	}
	return peer
}
func (api *API) WithTrustedProxyNetworks(networks []netip.Prefix) *API {
	api.trustedProxyNetworks = networks
	return api
}
func (api *API) WithAdminNetworks(networks []netip.Prefix) *API {
	api.adminNetworks = networks
	return api
}

func (api *API) productionSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := api.resolveClientIP(r)
		r = r.WithContext(context.WithValue(r.Context(), clientIPContext, client))
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'self'; form-action 'self'")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		} else if ip, e := netip.ParseAddr(peerIP(r)); e == nil && api.trustProxyHeaders && containsIP(api.trustedProxyNetworks, ip) && r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		for _, prefix := range []string{"/admin/", "/agent/", "/developer/", "/user/", "/v1/"} {
			if strings.HasPrefix(r.URL.Path, prefix) {
				h.Set("Cache-Control", "no-store")
				break
			}
		}
		protected := r.URL.Path == api.adminPath || strings.HasPrefix(r.URL.Path, api.adminPath+"/") || strings.HasPrefix(r.URL.Path, "/admin/")
		if protected && len(api.adminNetworks) > 0 {
			ip, e := netip.ParseAddr(client)
			if e != nil || !containsIP(api.adminNetworks, ip) {
				writeError(w, r, service.AppError{Code: "ADMIN_NETWORK_DENIED", Message: "management access is restricted to configured networks", HTTPStatus: 403})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
