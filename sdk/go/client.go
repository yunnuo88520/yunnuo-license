package ynlicense

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

type Client struct {
	baseURL   string
	appKey    string
	http      *http.Client
	userAgent string
}

type Option func(*Client)

func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.http = client
		}
	}
}

func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		if value := strings.TrimSpace(userAgent); value != "" {
			c.userAgent = value
		}
	}
}

func NewClient(baseURL, appKey string, options ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	appKey = strings.TrimSpace(appKey)
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || appKey == "" {
		return nil, errors.New("ynlicense: base URL and app key are required")
	}
	client := &Client{
		baseURL:   baseURL,
		appKey:    appKey,
		http:      &http.Client{Timeout: 10 * time.Second},
		userAgent: "yn-license-go/1.0",
	}
	for _, option := range options {
		option(client)
	}
	return client, nil
}

func (c *Client) Activate(ctx context.Context, input ActivateRequest) (LicenseResponse, error) {
	var result LicenseResponse
	err := c.post(ctx, "/v1/licenses/activate", struct {
		AppKey string `json:"app_key"`
		ActivateRequest
	}{c.appKey, input}, &result)
	return result, err
}

func (c *Client) Verify(ctx context.Context, input VerifyRequest) (LicenseResponse, error) {
	var result LicenseResponse
	err := c.post(ctx, "/v1/licenses/verify", struct {
		AppKey string `json:"app_key"`
		VerifyRequest
	}{c.appKey, input}, &result)
	return result, err
}

func (c *Client) Heartbeat(ctx context.Context, input HeartbeatRequest) (HeartbeatResponse, error) {
	var result HeartbeatResponse
	err := c.post(ctx, "/v1/licenses/heartbeat", struct {
		AppKey string `json:"app_key"`
		HeartbeatRequest
	}{c.appKey, input}, &result)
	return result, err
}

func (c *Client) Renew(ctx context.Context, input RenewRequest) (LicenseResponse, error) {
	var result LicenseResponse
	err := c.post(ctx, "/v1/licenses/renew", struct {
		AppKey string `json:"app_key"`
		RenewRequest
	}{c.appKey, input}, &result)
	return result, err
}

func (c *Client) Unbind(ctx context.Context, input UnbindRequest) (UnbindResponse, error) {
	var result UnbindResponse
	err := c.post(ctx, "/v1/licenses/unbind", struct {
		AppKey string `json:"app_key"`
		UnbindRequest
	}{c.appKey, input}, &result)
	return result, err
}

// ProductKey 描述公钥环中的一个版本。
type ProductKey struct {
	ProductID    string    `json:"product_id"`
	KeyVersion   int       `json:"key_version"`
	PublicKeyPEM string    `json:"public_key_pem"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProductKeyRing 是 /v1/products/keys 返回的产品公钥环，含全部历史版本。
type ProductKeyRing struct {
	ProductID      string       `json:"product_id"`
	ProductCode    string       `json:"product_code"`
	AppKey         string       `json:"app_key"`
	CurrentVersion int          `json:"current_version"`
	Keys           []ProductKey `json:"keys"`
}

// ProductKeys 拉取产品公钥环（含历史版本），供密钥环离线验签器使用。
func (c *Client) ProductKeys(ctx context.Context, appKey string) (ProductKeyRing, error) {
	key := strings.TrimSpace(appKey)
	if key == "" {
		key = c.appKey
	}
	var ring ProductKeyRing
	if err := c.get(ctx, "/v1/products/keys?app_key="+url.QueryEscape(key), &ring); err != nil {
		return ProductKeyRing{}, err
	}
	return ring, nil
}

// OfflineVerifierFromKeyRing 拉取公钥环并构造按 key_version 选钥的离线验签器。
func (c *Client) OfflineVerifierFromKeyRing(ctx context.Context, appKey, productCode string, options ...VerifierOption) (*OfflineVerifier, error) {
	ring, err := c.ProductKeys(ctx, appKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(productCode) == "" {
		productCode = ring.ProductCode
	}
	keys := make(map[int]string, len(ring.Keys))
	for _, entry := range ring.Keys {
		if entry.PublicKeyPEM != "" {
			keys[entry.KeyVersion] = entry.PublicKeyPEM
		}
	}
	return NewKeyRingOfflineVerifier(keys, productCode, options...)
}

func (c *Client) get(ctx context.Context, path string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("ynlicense: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ynlicense: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("ynlicense: read response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return errors.New("ynlicense: response exceeds 2 MiB limit")
	}
	data, err := decodeEnvelope(raw, resp.StatusCode)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return fmt.Errorf("ynlicense: decode response: %w", err)
		}
	}
	return nil
}

// decodeEnvelope 解析统一响应信封：非 2xx 或 success=false 时返回 APIError。
func decodeEnvelope(raw []byte, statusCode int) (json.RawMessage, error) {
	var envelope struct {
		Success   bool            `json:"success"`
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
		Error     *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("ynlicense: decode response (HTTP %d): %w", statusCode, err)
	}
	if statusCode < 200 || statusCode >= 300 || !envelope.Success {
		code, message := "HTTP_ERROR", http.StatusText(statusCode)
		if envelope.Error != nil {
			code, message = envelope.Error.Code, envelope.Error.Message
		}
		return nil, &APIError{Code: code, Message: message, HTTPStatus: statusCode, RequestID: envelope.RequestID}
	}
	return envelope.Data, nil
}

func (c *Client) post(ctx context.Context, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("ynlicense: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ynlicense: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ynlicense: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("ynlicense: read response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return errors.New("ynlicense: response exceeds 2 MiB limit")
	}
	data, err := decodeEnvelope(raw, resp.StatusCode)
	if err != nil {
		return err
	}
	if output == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("ynlicense: decode response data: %w", err)
	}
	return nil
}
