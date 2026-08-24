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
