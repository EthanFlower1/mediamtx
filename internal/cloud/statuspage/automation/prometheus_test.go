package automation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/statuspage/automation"
)

func TestHTTPPrometheusQuerier_VectorResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [{
					"metric": {"__name__": "up", "job": "cloud-apiserver"},
					"value": [1712700000, "1"]
				}]
			}
		}`))
	}))
	defer srv.Close()

	q := automation.NewHTTPPrometheusQuerier(srv.URL)
	val, err := q.Query(context.Background(), `up{job="cloud-apiserver"}`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if val != 1.0 {
		t.Errorf("expected 1.0, got %v", val)
	}
}

func TestHTTPPrometheusQuerier_ScalarResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "scalar",
				"result": [1712700000, "0.95"]
			}
		}`))
	}))
	defer srv.Close()

	q := automation.NewHTTPPrometheusQuerier(srv.URL)
	val, err := q.Query(context.Background(), "scalar(up)")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if val != 0.95 {
		t.Errorf("expected 0.95, got %v", val)
	}
}

func TestHTTPPrometheusQuerier_EmptyVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": []
			}
		}`))
	}))
	defer srv.Close()

	q := automation.NewHTTPPrometheusQuerier(srv.URL)
	_, err := q.Query(context.Background(), `up{job="nonexistent"}`)
	if err == nil {
		t.Fatal("expected error for empty result")
	}
}

func TestHTTPPrometheusQuerier_AmbiguousVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{"metric": {}, "value": [1712700000, "1"]},
					{"metric": {}, "value": [1712700000, "0"]}
				]
			}
		}`))
	}))
	defer srv.Close()

	q := automation.NewHTTPPrometheusQuerier(srv.URL)
	_, err := q.Query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for ambiguous vector")
	}
}

func TestHTTPPrometheusQuerier_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	q := automation.NewHTTPPrometheusQuerier(srv.URL)
	_, err := q.Query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestHTTPPrometheusQuerier_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status": "error", "errorType": "bad_data", "error": "invalid query"}`))
	}))
	defer srv.Close()

	q := automation.NewHTTPPrometheusQuerier(srv.URL)
	_, err := q.Query(context.Background(), "bad{")
	if err == nil {
		t.Fatal("expected error for error status")
	}
}
