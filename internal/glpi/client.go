// Package glpi is a minimal client for the GLPI legacy REST API (apirest.php).
package glpi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Item is one GLPI object as returned by the API. Numbers decode as json.Number.
type Item map[string]any

// Config holds connection settings.
type Config struct {
	BaseURL   string // e.g. https://glpi.example.com/apirest.php
	AppToken  string // optional
	UserToken string
	HTTP      *http.Client
}

// Client talks to one GLPI instance and keeps a lazily created session.
type Client struct {
	cfg Config

	mu      sync.Mutex
	session string
}

// APIError is a non-2xx answer. GLPI encodes errors as ["ERROR_CODE", "message"].
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("glpi: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// IsNotFound reports whether err is a 404 from GLPI.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{cfg: cfg}
}

func (c *Client) Create(ctx context.Context, itemtype string, input Item) (int64, error) {
	var out struct {
		ID json.Number `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/"+itemtype, nil, map[string]any{"input": input}, &out); err != nil {
		return 0, err
	}
	return out.ID.Int64()
}

func (c *Client) Get(ctx context.Context, itemtype string, id int64) (Item, error) {
	q := url.Values{"get_hateoas": {"false"}}
	var item Item
	if err := c.do(ctx, http.MethodGet, "/"+itemtype+"/"+strconv.FormatInt(id, 10), q, nil, &item); err != nil {
		return nil, err
	}
	return item, nil
}

func (c *Client) Update(ctx context.Context, itemtype string, id int64, input Item) error {
	var out []map[string]any
	if err := c.do(ctx, http.MethodPut, "/"+itemtype+"/"+strconv.FormatInt(id, 10), nil, map[string]any{"input": input}, &out); err != nil {
		return err
	}
	return checkBulk(out)
}

// Delete purges the item instead of moving it to the trash bin.
func (c *Client) Delete(ctx context.Context, itemtype string, id int64) error {
	q := url.Values{"force_purge": {"true"}}
	var out []map[string]any
	if err := c.do(ctx, http.MethodDelete, "/"+itemtype+"/"+strconv.FormatInt(id, 10), q, nil, &out); err != nil {
		return err
	}
	return checkBulk(out)
}

// FindByName returns all items whose name equals name exactly. GLPI's searchText is a
// substring match, so the result is filtered client-side.
func (c *Client) FindByName(ctx context.Context, itemtype, name string) ([]Item, error) {
	q := url.Values{
		"searchText[name]": {name},
		"range":            {"0-1000"},
		"get_hateoas":      {"false"},
	}
	var items []Item
	if err := c.do(ctx, http.MethodGet, "/"+itemtype, q, nil, &items); err != nil {
		return nil, err
	}
	exact := items[:0]
	for _, it := range items {
		if s, _ := it["name"].(string); s == name {
			exact = append(exact, it)
		}
	}
	return exact, nil
}

func checkBulk(out []map[string]any) error {
	for _, row := range out {
		for k, v := range row {
			if ok, isBool := v.(bool); isBool && !ok {
				msg, _ := row["message"].(string)
				return fmt.Errorf("glpi: item %s rejected: %s", k, msg)
			}
		}
	}
	return nil
}

func (c *Client) initSession(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/initSession", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "user_token "+c.cfg.UserToken)
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.AppToken != "" {
		req.Header.Set("App-Token", c.cfg.AppToken)
	}
	var out struct {
		SessionToken string `json:"session_token"`
	}
	if err := c.send(req, &out); err != nil {
		return "", fmt.Errorf("initSession: %w", err)
	}
	return out.SessionToken, nil
}

func (c *Client) sessionToken(ctx context.Context, stale string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != "" && c.session != stale {
		return c.session, nil
	}
	tok, err := c.initSession(ctx)
	if err != nil {
		return "", err
	}
	c.session = tok
	return tok, nil
}

// do performs an authenticated call and retries once with a fresh session on 401.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	stale := ""
	for attempt := 0; ; attempt++ {
		tok, err := c.sessionToken(ctx, stale)
		if err != nil {
			return err
		}
		u := c.cfg.BaseURL + path
		if len(query) > 0 {
			u += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Session-Token", tok)
		if c.cfg.AppToken != "" {
			req.Header.Set("App-Token", c.cfg.AppToken)
		}
		err = c.send(req, out)
		var apiErr *APIError
		if attempt == 0 && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized {
			stale = tok
			continue
		}
		return err
	}
}

func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseError(resp.StatusCode, raw)
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("glpi: decoding response: %w", err)
	}
	return nil
}

func parseError(status int, raw []byte) *APIError {
	e := &APIError{Status: status, Message: strings.TrimSpace(string(raw))}
	var pair []string
	if json.Unmarshal(raw, &pair) == nil && len(pair) > 0 {
		e.Code = pair[0]
		if len(pair) > 1 {
			e.Message = pair[1]
		}
	}
	return e
}
