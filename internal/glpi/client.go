package glpi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxResponseBytes = 32 << 20
	maxErrorBytes    = 512
	errItemNotFound  = "ERROR_ITEM_NOT_FOUND"
)

type Item map[string]any

type Config struct {
	BaseURL   string
	AppToken  string
	UserToken string

	PurgeOnDestroy bool
}

type Client struct {
	cfg  Config
	http *http.Client

	mu      sync.Mutex
	session string
}

type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("glpi: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound && apiErr.Code == errItemNotFound
}

func New(cfg Config) *Client {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: 60 * time.Second,

			CheckRedirect: refuseRedirects,
		},
	}
}

func refuseRedirects(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func (c *Client) Create(ctx context.Context, itemtype string, input Item) (int64, error) {
	var out struct {
		ID json.Number `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/"+itemtype, nil, map[string]any{"input": input}, &out); err != nil {
		return 0, err
	}
	id, err := out.ID.Int64()
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("glpi: create %s returned no usable id (%q)", itemtype, out.ID)
	}
	return id, nil
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

func (c *Client) Delete(ctx context.Context, itemtype string, id int64) error {
	q := url.Values{"force_purge": {strconv.FormatBool(c.cfg.PurgeOnDestroy)}}
	var out []map[string]any
	if err := c.do(ctx, http.MethodDelete, "/"+itemtype+"/"+strconv.FormatInt(id, 10), q, nil, &out); err != nil {
		return err
	}
	return checkBulk(out)
}

func searchSegment(name string) string {
	best := ""
	for _, part := range strings.FieldsFunc(name, isUnreliableSearchRune) {
		if len(part) > len(best) {
			best = part
		}
	}
	return best
}

func isUnreliableSearchRune(r rune) bool { return strings.ContainsRune(`&<>"'`, r) }

func (c *Client) FindByName(ctx context.Context, itemtype, name string) ([]Item, error) {
	q := url.Values{
		"range":       {"0-1000"},
		"get_hateoas": {"false"},
	}
	if seg := searchSegment(name); seg != "" {
		q.Set("searchText[name]", seg)
	}
	var candidates []Item
	if err := c.do(ctx, http.MethodGet, "/"+itemtype, q, nil, &candidates); err != nil {
		return nil, err
	}
	exact := candidates[:0]
	for _, it := range candidates {
		if s, _ := it["name"].(string); html.UnescapeString(s) == name {
			exact = append(exact, it)
		}
	}
	return exact, nil
}

func checkBulk(out []map[string]any) error {
	for _, row := range out {
		for _, k := range sortedKeys(row) {
			if ok, isBool := row[k].(bool); isBool && !ok {
				msg, _ := row["message"].(string)
				return fmt.Errorf("glpi: item %s rejected: %s", k, msg)
			}
		}
	}
	return nil
}

func sortedKeys(row map[string]any) []string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
		return "", fmt.Errorf("initSession (the url must be the apirest.php endpoint): %w", err)
	}
	if out.SessionToken == "" {
		return "", errors.New("initSession: response contained no session_token")
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

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	payload, err := marshalBody(body)
	if err != nil {
		return err
	}
	tok, err := c.attempt(ctx, method, path, query, payload, out, "")
	if tok != "" && isUnauthorized(err) {
		_, err = c.attempt(ctx, method, path, query, payload, out, tok)
	}
	return err
}

func marshalBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	return json.Marshal(body)
}

func isUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized
}

func (c *Client) attempt(ctx context.Context, method, path string, query url.Values, payload []byte, out any, stale string) (string, error) {
	tok, err := c.sessionToken(ctx, stale)
	if err != nil {
		return "", err
	}
	u := c.cfg.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
	if err != nil {
		return tok, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Session-Token", tok)
	if c.cfg.AppToken != "" {
		req.Header.Set("App-Token", c.cfg.AppToken)
	}
	return tok, c.send(req, out)
}

func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := readBody(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseError(resp, raw)
	}
	return decodeJSON(raw, out)
}

func readBody(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err == nil && len(raw) > maxResponseBytes {
		err = fmt.Errorf("glpi: response larger than %d bytes", maxResponseBytes)
	}
	return raw, err
}

func decodeJSON(raw []byte, out any) error {
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

func parseError(resp *http.Response, raw []byte) *APIError {
	e := &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw))}
	var codeAndMessage []string
	if json.Unmarshal(raw, &codeAndMessage) == nil && len(codeAndMessage) > 0 {
		e.Code = codeAndMessage[0]
		if len(codeAndMessage) > 1 {
			e.Message = codeAndMessage[1]
		}
	}
	if len(e.Message) > maxErrorBytes {
		e.Message = e.Message[:maxErrorBytes] + "..."
	}
	if loc := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode < 400 && loc != "" {
		e.Message = "redirected to " + loc + "; set url to the final apirest.php address"
	}
	return e
}
