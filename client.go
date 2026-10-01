package sudhanva

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

const (
	// Version is the semantic version of this SDK release line.
	Version        = "0.2.0"
	defaultBaseURL = "https://sudhanva.me/api/v1"
	userAgent      = "sudhanva-go/" + Version
)

// Client calls the stable public sudhanva.me API.
type Client struct {
	baseURL    *url.URL
	siteURL    *url.URL
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client) error

// WithBaseURL replaces the production API base URL. Use it for tests and proxies.
func WithBaseURL(rawURL string) Option {
	return func(client *Client) error {
		parsed, err := url.Parse(strings.TrimRight(rawURL, "/"))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("base URL must be an absolute HTTP(S) URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("base URL must use HTTP or HTTPS")
		}
		client.baseURL = parsed
		client.siteURL = &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}
		return nil
	}
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(client *Client) error {
		if httpClient == nil {
			return errors.New("HTTP client cannot be nil")
		}
		client.httpClient = httpClient
		return nil
	}
}

// NewClient creates a synchronous API client.
func NewClient(options ...Option) (*Client, error) {
	baseURL, _ := url.Parse(defaultBaseURL)
	client := &Client{
		baseURL:    baseURL,
		siteURL:    &url.URL{Scheme: baseURL.Scheme, Host: baseURL.Host},
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, option := range options {
		if err := option(client); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// APIError is a structured non-success response from the API. It decodes both the standard
// {"error": {...}} envelope and RFC 9457 application/problem+json bodies; Body keeps the raw response.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Hint       string
	DocsURL    string
	Body       json.RawMessage
}

func (err *APIError) Error() string {
	return fmt.Sprintf("%d %s: %s", err.StatusCode, err.Code, err.Message)
}

type ProfileResponse struct {
	Profile Profile `json:"profile"`
}

type Profile struct {
	Name           string       `json:"name"`
	JobTitle       string       `json:"jobTitle"`
	Specialization string       `json:"specialization"`
	Location       string       `json:"location"`
	URL            string       `json:"url"`
	Email          string       `json:"email"`
	WorksFor       Organization `json:"worksFor"`
	KnowsAbout     []string     `json:"knowsAbout"`
	SameAs         []string     `json:"sameAs"`
}

// Organization is the employer named in the published profile.
type Organization struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Post struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	PublishedAt string   `json:"publishedAt"`
	UpdatedAt   string   `json:"updatedAt"`
	Tags        []string `json:"tags"`
	Category    string   `json:"category"`
	URL         string   `json:"url"`
}

type PostsResponse struct {
	Count      int     `json:"count"`
	Total      int     `json:"total"`
	NextCursor *string `json:"next_cursor"`
	Posts      []Post  `json:"posts"`
}

type PostResponse struct {
	Post Post `json:"post"`
}

type PostsOptions struct {
	Limit  int
	Tag    string
	Cursor string
}

type BatchOperation struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type BatchResult struct {
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

type BatchResponse struct {
	Count   int           `json:"count"`
	Results []BatchResult `json:"results"`
}

type ProfileInsightRequest struct {
	Audience string   `json:"audience"`
	Focus    []string `json:"focus,omitempty"`
}

type ProfileInsightJob struct {
	JobID     string          `json:"job_id"`
	Status    string          `json:"status"`
	StatusURL string          `json:"status_url"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	ExpiresAt string          `json:"expires_at"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     json.RawMessage `json:"error,omitempty"`
}

// Profile returns the currently published professional profile.
func (client *Client) Profile(ctx context.Context) (*ProfileResponse, error) {
	query := url.Values{"locale": {"en"}}
	var result ProfileResponse
	err := client.do(ctx, http.MethodGet, client.apiURL("/profile", query), nil, nil, &result)
	return &result, err
}

// Posts returns a filtered page of published articles.
func (client *Client) Posts(ctx context.Context, options PostsOptions) (*PostsResponse, error) {
	limit := options.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("limit must be between 1 and 100")
	}
	query := url.Values{"limit": {fmt.Sprint(limit)}}
	if options.Tag != "" {
		query.Set("tag", options.Tag)
	}
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	var result PostsResponse
	err := client.do(ctx, http.MethodGet, client.apiURL("/posts", query), nil, nil, &result)
	return &result, err
}

// Post returns metadata for one canonical article slug.
func (client *Client) Post(ctx context.Context, slug string) (*PostResponse, error) {
	if slug == "" {
		return nil, errors.New("slug is required")
	}
	var result PostResponse
	err := client.do(ctx, http.MethodGet, client.apiURL("/posts/"+url.PathEscape(slug), nil), nil, nil, &result)
	return &result, err
}

// Batch executes between one and twenty allowlisted public reads.
func (client *Client) Batch(ctx context.Context, operations []BatchOperation) (*BatchResponse, error) {
	if len(operations) < 1 || len(operations) > 20 {
		return nil, errors.New("operations must contain between 1 and 20 items")
	}
	var result BatchResponse
	err := client.do(ctx, http.MethodPost, client.apiURL("/batch", nil), map[string]any{"operations": operations}, nil, &result)
	return &result, err
}

// CreateProfileInsight creates an idempotent, short-lived profile-insight job.
func (client *Client) CreateProfileInsight(ctx context.Context, request ProfileInsightRequest, idempotencyKey string) (*ProfileInsightJob, error) {
	if idempotencyKey == "" {
		return nil, errors.New("idempotency key is required")
	}
	var result ProfileInsightJob
	headers := http.Header{"Idempotency-Key": {idempotencyKey}}
	err := client.do(ctx, http.MethodPost, client.apiURL("/profile-insights", nil), request, headers, &result)
	return &result, err
}

// ProfileInsight returns the current state of a profile-insight job.
func (client *Client) ProfileInsight(ctx context.Context, jobID string) (*ProfileInsightJob, error) {
	if jobID == "" {
		return nil, errors.New("job ID is required")
	}
	var result ProfileInsightJob
	err := client.do(ctx, http.MethodGet, client.apiURL("/profile-insights/"+url.PathEscape(jobID), nil), nil, nil, &result)
	return &result, err
}

// WaitProfileInsight polls until the job succeeds, fails, or the context is canceled.
func (client *Client) WaitProfileInsight(ctx context.Context, jobID string, pollInterval time.Duration) (*ProfileInsightJob, error) {
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	for {
		job, err := client.ProfileInsight(ctx, jobID)
		if err != nil {
			return nil, err
		}
		if job.Status == "succeeded" || job.Status == "failed" {
			return job, nil
		}

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type AskOptions struct {
	Limit int
	Mode  string
}

// Ask performs NLWeb conversational search and returns the decoded response.
func (client *Client) Ask(ctx context.Context, text string, options AskOptions) (map[string]any, error) {
	if text == "" {
		return nil, errors.New("text is required")
	}
	limit := options.Limit
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 20 {
		return nil, errors.New("limit must be between 1 and 20")
	}
	mode := options.Mode
	if mode == "" {
		mode = "list"
	}
	body := map[string]any{
		"query":  map[string]any{"text": text, "site": client.siteURL.String(), "limit": limit},
		"prefer": map[string]any{"streaming": false, "response_format": "conversational_search", "mode": mode},
		"meta":   map[string]any{"version": "0.55"},
	}
	result := make(map[string]any)
	err := client.do(ctx, http.MethodPost, client.siteEndpoint("/ask"), body, nil, &result)
	return result, err
}

func (client *Client) apiURL(path string, query url.Values) string {
	endpoint := *client.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/" + strings.TrimLeft(path, "/")
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	return endpoint.String()
}

func (client *Client) siteEndpoint(path string) string {
	endpoint := *client.siteURL
	endpoint.Path = "/" + strings.TrimLeft(path, "/")
	return endpoint.String()
}

func (client *Client) do(ctx context.Context, method, endpoint string, body any, headers http.Header, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return newAPIError(response.StatusCode, data)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// newAPIError decodes the standard error envelope or an RFC 9457 problem. Unexpected bodies, such as
// arrays, plain text, or a string-valued "error", still produce an APIError with fallback values.
func newAPIError(status int, data []byte) *APIError {
	var body map[string]any
	_ = json.Unmarshal(data, &body)
	fields := body
	nested, isObject := body["error"].(map[string]any)
	if isObject {
		fields = nested
	}

	text := func(object map[string]any, key string) string {
		value, _ := object[key].(string)
		return value
	}
	code := firstNonEmpty(text(fields, "code"), text(body, "code"))
	message := firstNonEmpty(text(fields, "message"), text(body, "message"))
	if code == "" && message == "" {
		title := text(body, "title")
		code = firstNonEmpty(problemCode(text(body, "type")), title)
		message = firstNonEmpty(text(body, "detail"), title)
	}
	if message == "" {
		message = text(body, "error")
	}

	return &APIError{
		StatusCode: status,
		Code:       firstNonEmpty(code, "api_error"),
		Message:    firstNonEmpty(message, "request failed"),
		Hint:       text(fields, "hint"),
		DocsURL:    text(fields, "docs_url"),
		Body:       data,
	}
}

// problemCode returns the last fragment or path segment of an RFC 9457 problem type URI.
func problemCode(problemType string) string {
	if problemType == "" || problemType == "about:blank" {
		return ""
	}
	base, fragment, _ := strings.Cut(problemType, "#")
	if fragment != "" {
		return fragment
	}
	base = strings.TrimRight(base, "/")
	return base[strings.LastIndex(base, "/")+1:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
