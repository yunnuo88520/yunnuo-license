package ynlicense

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// OpenInstallation publishes one complete, private identity even on concurrent
// first starts. The containing directory must be a persistent private volume.
func OpenInstallation(dir, device string) (ClientInstance, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ClientInstance{}, err
	}
	path := filepath.Join(dir, "instance.json")
	read := func() (ClientInstance, error) {
		var v ClientInstance
		info, e := os.Lstat(path)
		if e != nil {
			return v, e
		}
		if !info.Mode().IsRegular() || info.Size() > 8192 {
			return v, errors.New("ynlicense: invalid installation file")
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return v, e
		}
		e = json.Unmarshal(raw, &v)
		if e != nil {
			return v, e
		}
		secret, e := hex.DecodeString(v.InstanceSecret)
		if len(v.InstanceID) < 16 || len(v.InstanceID) > 64 || e != nil || len(secret) != 32 {
			return v, errors.New("ynlicense: invalid installation identity")
		}
		if device != "" && v.DeviceID != device {
			return v, errors.New("ynlicense: the configured device changed; use the controlled rebind flow")
		}
		return v, nil
	}
	if v, e := read(); e == nil {
		return v, nil
	} else if !os.IsNotExist(e) {
		return v, e
	}
	random := make([]byte, 64)
	if _, e := rand.Read(random); e != nil {
		return ClientInstance{}, e
	}
	if device == "" {
		device = hex.EncodeToString(random[48:])
	}
	v := ClientInstance{InstanceID: hex.EncodeToString(random[:16]), InstanceSecret: hex.EncodeToString(random[16:48]), DeviceID: device}
	raw, e := json.Marshal(v)
	if e != nil {
		return v, e
	}
	file, e := os.CreateTemp(dir, ".identity-*")
	if e != nil {
		return v, e
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, e = file.Write(raw); e != nil {
		file.Close()
		return v, e
	}
	if e = file.Sync(); e != nil {
		file.Close()
		return v, e
	}
	if e = file.Close(); e != nil {
		return v, e
	}
	if e = os.Link(temp, path); e != nil && !os.IsExist(e) {
		return v, e
	}
	return read()
}

// BindingForHTTPRequest replaces configured domain claims with the actual
// request host. Forwarded hosts are used only for explicitly trusted proxies.
func BindingForHTTPRequest(in VerifyRequest, r *http.Request, trusted []netip.Prefix) (VerifyRequest, error) {
	needs := in.BindMode == "domain" || in.BindMode == "domain_wildcard" || in.BindMode == "composite"
	if !needs {
		return in, nil
	}
	if r == nil {
		return in, errors.New("ynlicense: actual HTTP request required")
	}
	host := r.Host
	remote, _, _ := net.SplitHostPort(r.RemoteAddr)
	addr, e := netip.ParseAddr(remote)
	if e == nil {
		for _, prefix := range trusted {
			if prefix.Contains(addr.Unmap()) && r.Header.Get("X-Forwarded-Host") != "" {
				host = r.Header.Get("X-Forwarded-Host")
				break
			}
		}
	}
	if host == "" || strings.ContainsAny(host, " \t\r\n,/@?#\\") {
		return in, errors.New("ynlicense: invalid request host")
	}
	parsed, e := url.Parse("http://" + host)
	if e != nil || parsed.User != nil || parsed.Hostname() == "" || parsed.Path != "" {
		return in, errors.New("ynlicense: invalid request host")
	}
	value := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if in.BindMode == "composite" {
		fields := in.Bindings
		if len(fields) == 0 {
			if e = json.Unmarshal([]byte(in.BindValue), &fields); e != nil {
				return in, e
			}
		}
		copy := map[string]string{}
		for k, v := range fields {
			if k == "domain" || k == "domain_wildcard" {
				v = value
			}
			copy[k] = v
		}
		raw, e := json.Marshal(copy)
		if e != nil {
			return in, e
		}
		in.BindValue = string(raw)
		in.Bindings = nil
	} else {
		in.BindValue = value
	}
	return in, nil
}
