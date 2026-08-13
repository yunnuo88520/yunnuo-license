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
		return fmt.Errorf("ynlicense: decode response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.Success {
		code, message := "HTTP_ERROR", http.StatusText(resp.StatusCode)
		if envelope.Error != nil {
			if envelope.Error.Code != "" {
				code = envelope.Error.Code
			}
			if envelope.Error.Message != "" {
				message = envelope.Error.Message
			}
		}
		return &APIError{Code: code, Message: message, HTTPStatus: resp.StatusCode, RequestID: envelope.RequestID}
	}
	if output == nil || len(envelope.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("ynlicense: decode response data: %w", err)
	}
	return nil
}
