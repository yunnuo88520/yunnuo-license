package ynlicense

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

type ProductRelease struct {
	ID            string         `json:"id"`
	ProductID     string         `json:"product_id"`
	Version       string         `json:"version"`
	Notes         string         `json:"notes"`
	Filename      string         `json:"filename"`
	Size          int64          `json:"size"`
	SHA256        string         `json:"sha256"`
	KeyVersion    int            `json:"key_version"`
	ManifestToken string         `json:"manifest_token"`
	Target        *ReleaseTarget `json:"target,omitempty"`
}
type CheckUpdateRequest struct {
	VerifyRequest
	CurrentVersion string `json:"current_version"`
}
type CheckUpdateResponse struct {
	Enabled         bool            `json:"enabled"`
	UpdateAvailable bool            `json:"update_available"`
	Release         *ProductRelease `json:"release,omitempty"`
}
type ReleaseExpectation struct {
	PublicKeyPEM   string
	ProductID      string
	Target         *ReleaseTarget
	CurrentVersion string
}
type releaseClaims struct {
	Type       string         `json:"type"`
	ReleaseID  string         `json:"release_id"`
	ProductID  string         `json:"product_id"`
	Version    string         `json:"version"`
	Size       int64          `json:"size"`
	SHA256     string         `json:"sha256"`
	KeyVersion int            `json:"key_version"`
	Target     *ReleaseTarget `json:"target"`
}

func VerifyReleaseManifest(r ProductRelease, expected ReleaseExpectation) error {
	key, e := parsePublicKey(expected.PublicKeyPEM)
	if e != nil {
		return e
	}
	c, e := verifySignedJSON[releaseClaims](key, r.ManifestToken)
	if e != nil {
		return e
	}
	if expected.ProductID == "" || c.Type != "product_release" || c.ProductID != expected.ProductID || c.ProductID != r.ProductID || c.ReleaseID != r.ID || c.Version != r.Version || c.Size != r.Size || c.SHA256 != r.SHA256 || c.KeyVersion != r.KeyVersion || !reflect.DeepEqual(c.Target, r.Target) || c.Size <= 0 || c.Size > 256<<20 {
		return errors.New("ynlicense: signed release identity, target or package does not match")
	}
	if c.Target != nil {
		if expected.Target == nil {
			return errors.New("ynlicense: explicit client target required")
		}
		a, b := c.Target, expected.Target
		for _, pair := range [][2]string{{a.OS, b.OS}, {a.Arch, b.Arch}, {a.Runtime, b.Runtime}} {
			if pair[0] != "any" && pair[0] != pair[1] {
				return errors.New("ynlicense: wrong client platform")
			}
		}
		channel := b.Channel
		if channel == "" {
			channel = "stable"
		}
		if a.Channel != channel || b.Kind != "" && a.Kind != b.Kind {
			return errors.New("ynlicense: wrong update channel or kind")
		}
		if a.MinVersion != "" || a.MaxVersion != "" {
			if !validSourceVersion(expected.CurrentVersion) || a.MinVersion != "" && compareSourceVersion(expected.CurrentVersion, a.MinVersion) < 0 || a.MaxVersion != "" && compareSourceVersion(expected.CurrentVersion, a.MaxVersion) > 0 {
				return errors.New("ynlicense: source version outside signed upgrade path")
			}
		}
	}
	return nil
}
func validSourceVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		n, e := strconv.Atoi(part)
		if e != nil || n < 0 || strconv.Itoa(n) != part {
			return false
		}
	}
	return true
}
func compareSourceVersion(a, b string) int {
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}
func VerifyReleasePackage(r ProductRelease, content []byte, expected ReleaseExpectation) error {
	if e := VerifyReleaseManifest(r, expected); e != nil {
		return e
	}
	sum := sha256.Sum256(content)
	if int64(len(content)) != r.Size || hex.EncodeToString(sum[:]) != r.SHA256 {
		return errors.New("ynlicense: release package checksum mismatch")
	}
	return nil
}
func (c *Client) CheckUpdate(ctx context.Context, in CheckUpdateRequest) (CheckUpdateResponse, error) {
	var out CheckUpdateResponse
	e := c.post(ctx, "/v1/updates/check", struct {
		AppKey string `json:"app_key"`
		CheckUpdateRequest
	}{c.appKey, in}, &out)
	return out, e
}
func (c *Client) AuthorizeUpdate(ctx context.Context, id string, in VerifyRequest) error {
	var out struct {
		Authorized bool   `json:"authorized"`
		ReleaseID  string `json:"release_id"`
	}
	err := c.post(ctx, "/v1/updates/"+url.PathEscape(id)+"/authorize", struct {
		AppKey string `json:"app_key"`
		VerifyRequest
	}{c.appKey, in}, &out)
	if err != nil {
		return err
	}
	if !out.Authorized || out.ReleaseID != id {
		return errors.New("ynlicense: exact release authorization was not granted")
	}
	return nil
}
func (c *Client) DownloadUpdate(ctx context.Context, r ProductRelease, in VerifyRequest, expected ReleaseExpectation) ([]byte, error) {
	if expected.Target == nil {
		expected.Target = in.Target
	}
	if expected.CurrentVersion == "" {
		expected.CurrentVersion = in.CurrentVersion
	}
	if e := VerifyReleaseManifest(r, expected); e != nil {
		return nil, e
	}
	payload, e := json.Marshal(struct {
		AppKey string `json:"app_key"`
		VerifyRequest
	}{c.appKey, in})
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/updates/"+url.PathEscape(r.ID)+"/download", bytes.NewReader(payload))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, e := c.http.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, e := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		if e != nil {
			return nil, e
		}
		_, e = decodeEnvelope(raw, resp.StatusCode)
		return nil, e
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, r.Size+1))
	if e != nil {
		return nil, e
	}
	if e = VerifyReleasePackage(r, data, expected); e != nil {
		return nil, e
	}
	if e = c.AuthorizeUpdate(ctx, r.ID, in); e != nil {
		return nil, e
	}
	return data, nil
}
func (c *Client) CardRequirements(ctx context.Context, card string) (map[string]any, error) {
	var out map[string]any
	e := c.post(ctx, "/v1/cards/requirements", map[string]string{"app_key": c.appKey, "card_code": card}, &out)
	return out, e
}
func (c *Client) AcquireSeat(ctx context.Context, in SeatRequest) (SeatLease, error) {
	var out SeatLease
	e := c.post(ctx, "/v1/licenses/seats/acquire", struct {
		AppKey string `json:"app_key"`
		SeatRequest
	}{c.appKey, in}, &out)
	return out, e
}
func (c *Client) ReleaseSeat(ctx context.Context, in VerifyRequest) error {
	return c.post(ctx, "/v1/licenses/seats/release", struct {
		AppKey string `json:"app_key"`
		VerifyRequest
	}{c.appKey, in}, nil)
}
func (c *Client) CreateClaimTicket(ctx context.Context, in VerifyRequest) (ClaimTicket, error) {
	var out ClaimTicket
	e := c.post(ctx, "/v1/licenses/claim-ticket", struct {
		AppKey string `json:"app_key"`
		VerifyRequest
	}{c.appKey, in}, &out)
	return out, e
}
