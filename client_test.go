package sudhanva

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVersionIdentifiesClient(t *testing.T) {
	if Version != "0.2.0" {
		t.Fatalf("unexpected SDK version: %s", Version)
	}
	if userAgent != "sudhanva-go/"+Version {
		t.Fatalf("user agent and SDK version differ: %s", userAgent)
	}
}

func TestPostsEncodesFiltersAndIdentifiesClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/posts" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.URL.Query().Get("limit") != "5" || request.URL.Query().Get("tag") != "machine-learning" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		if request.UserAgent() != userAgent {
			t.Fatalf("unexpected user agent: %s", request.UserAgent())
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"count":0,"total":0,"posts":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL + "/api/v1"))
	if err != nil {
		t.Fatal(err)
	}
	posts, err := client.Posts(context.Background(), PostsOptions{Limit: 5, Tag: "machine-learning"})
	if err != nil {
		t.Fatal(err)
	}
	if posts.Count != 0 || len(posts.Posts) != 0 {
		t.Fatalf("unexpected response: %+v", posts)
	}
}

func TestInsightAndWaitPreserveIdempotency(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/profile-insights":
			if request.Header.Get("Idempotency-Key") != "go-test-123" {
				t.Fatalf("missing idempotency key")
			}
			var body ProfileInsightRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Audience != "agent" {
				t.Fatalf("unexpected body: %+v", body)
			}
			writer.WriteHeader(http.StatusAccepted)
			_, _ = writer.Write([]byte(`{"job_id":"pi_test","status":"queued"}`))
		case request.URL.Path == "/api/v1/profile-insights/pi_test":
			polls++
			status := "running"
			if polls > 1 {
				status = "succeeded"
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"job_id": "pi_test", "status": status})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL + "/api/v1"))
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.CreateProfileInsight(context.Background(), ProfileInsightRequest{Audience: "agent"}, "go-test-123")
	if err != nil || job.Status != "queued" {
		t.Fatalf("create: job=%+v err=%v", job, err)
	}
	job, err = client.WaitProfileInsight(context.Background(), job.JobID, time.Millisecond)
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("wait: job=%+v err=%v", job, err)
	}
}

func TestAskUsesSiteRoot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ask" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL + "/api/v1"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Ask(context.Background(), "Kubernetes", AskOptions{Limit: 3, Mode: "summarize"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result["results"]; !ok {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestStructuredErrorsAreExposed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"error":{"code":"not_found","message":"Missing"}}`))
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL + "/api/v1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Post(context.Background(), "missing")
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiError.StatusCode != 404 || apiError.Code != "not_found" {
		t.Fatalf("unexpected APIError: %+v", apiError)
	}
}

func errorFrom(t *testing.T, status int, contentType, body string, call func(*Client) error) *APIError {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", contentType)
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL + "/api/v1"))
	if err != nil {
		t.Fatal(err)
	}
	err = call(client)
	var apiError *APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiError.StatusCode != status || string(apiError.Body) != body {
		t.Fatalf("unexpected status or body: %+v", apiError)
	}
	return apiError
}

func TestErrorEnvelopeKeepsHintAndDocsURL(t *testing.T) {
	body := `{"error":{"code":"POST_NOT_FOUND","message":"No published post exists.","hint":"List published posts first.","docs_url":"https://sudhanva.me/developers/"}}`
	apiError := errorFrom(t, http.StatusNotFound, "application/json", body, func(client *Client) error {
		_, err := client.Post(context.Background(), "missing")
		return err
	})
	if apiError.Code != "POST_NOT_FOUND" || apiError.Message != "No published post exists." ||
		apiError.Hint != "List published posts first." || apiError.DocsURL != "https://sudhanva.me/developers/" {
		t.Fatalf("unexpected APIError: %+v", apiError)
	}
}

func TestProblemDetailsAreExposed(t *testing.T) {
	body := `{"type":"https://sudhanva.me/docs/profile-insights/#idempotency-key-reuse","title":"Idempotency-Key reused","status":422,"detail":"This key was already used with a different request body.","instance":"/api/v1/profile-insights"}`
	apiError := errorFrom(t, http.StatusUnprocessableEntity, "application/problem+json", body, func(client *Client) error {
		_, err := client.CreateProfileInsight(context.Background(), ProfileInsightRequest{Audience: "agent"}, "go-test-123")
		return err
	})
	if apiError.Code != "idempotency-key-reuse" || apiError.Message != "This key was already used with a different request body." {
		t.Fatalf("unexpected APIError: %+v", apiError)
	}
}

func TestProblemWithoutDetailFallsBackToTitle(t *testing.T) {
	body := `{"type":"about:blank","title":"Service Unavailable","status":503}`
	apiError := errorFrom(t, http.StatusServiceUnavailable, "application/problem+json", body, func(client *Client) error {
		_, err := client.ProfileInsight(context.Background(), "pi_test")
		return err
	})
	if apiError.Code != "Service Unavailable" || apiError.Message != "Service Unavailable" {
		t.Fatalf("unexpected APIError: %+v", apiError)
	}
}

func TestUnexpectedErrorBodiesStillReturnAPIError(t *testing.T) {
	for _, body := range []string{`["unexpected"]`, `"oops"`, `null`, `not json`, ``} {
		apiError := errorFrom(t, http.StatusBadGateway, "application/json", body, func(client *Client) error {
			_, err := client.Profile(context.Background())
			return err
		})
		if apiError.Code != "api_error" || apiError.Message != "request failed" {
			t.Fatalf("body %q: unexpected APIError: %+v", body, apiError)
		}
	}
	apiError := errorFrom(t, http.StatusBadGateway, "application/json", `{"error":"Bad gateway"}`, func(client *Client) error {
		_, err := client.Profile(context.Background())
		return err
	})
	if apiError.Code != "api_error" || apiError.Message != "Bad gateway" {
		t.Fatalf("unexpected APIError: %+v", apiError)
	}
}

func TestResponsesDecodePublishedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/profile":
			_, _ = writer.Write([]byte(`{"profile":{"name":"Sudhanva Narayana","worksFor":{"name":"Example Co","url":"https://example.test/"}}}`))
		case "/api/v1/batch":
			_, _ = writer.Write([]byte(`{"count":1,"results":[{"id":"profile","status":200,"body":{}}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL + "/api/v1"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := client.Profile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.Profile.WorksFor != (Organization{Name: "Example Co", URL: "https://example.test/"}) {
		t.Fatalf("unexpected worksFor: %+v", profile.Profile.WorksFor)
	}
	batch, err := client.Batch(context.Background(), []BatchOperation{{ID: "profile", Method: "GET", Path: "/profile"}})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Count != 1 || len(batch.Results) != 1 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
}
