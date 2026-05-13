package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContext7Client_SearchAndDocs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ctx7-test" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v2/libs/search":
			if r.URL.Query().Get("libraryName") != "next.js" {
				t.Fatalf("libraryName = %q", r.URL.Query().Get("libraryName"))
			}
			if r.URL.Query().Get("query") != "middleware auth" {
				t.Fatalf("query = %q", r.URL.Query().Get("query"))
			}
			_, _ = w.Write([]byte(`{
				"results": [
					{"id": "/vercel/next.js", "title": "Next.js", "description": "React framework", "trustScore": 10, "benchmarkScore": 95.5}
				],
				"searchFilterApplied": false
			}`))
		case "/api/v2/context":
			if r.URL.Query().Get("libraryId") != "/vercel/next.js" {
				t.Fatalf("libraryId = %q", r.URL.Query().Get("libraryId"))
			}
			if r.URL.Query().Get("type") != "json" {
				t.Fatalf("type = %q", r.URL.Query().Get("type"))
			}
			_, _ = w.Write([]byte(`{
				"codeSnippets": [{
					"codeTitle": "Middleware",
					"codeDescription": "Auth middleware",
					"codeLanguage": "typescript",
					"codeId": "https://github.com/vercel/next.js/docs/middleware.mdx#_snippet_0",
					"codeList": [{"language": "typescript", "code": "export function middleware() {}"}]
				}],
				"infoSnippets": [{
					"pageId": "https://github.com/vercel/next.js/docs/middleware.mdx",
					"breadcrumb": "Routing > Middleware",
					"content": "Middleware runs before a request completes."
				}]
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := NewContext7Client(Context7Endpoint{Name: "ctx", APIURL: ts.URL, APIKey: "ctx7-test"})
	search, err := client.SearchLibraries(context.Background(), Context7LibrarySearchRequest{LibraryName: "next.js", Query: "middleware auth"})
	if err != nil {
		t.Fatalf("SearchLibraries failed: %v", err)
	}
	if search.Results[0].ID != "/vercel/next.js" {
		t.Fatalf("library id = %q", search.Results[0].ID)
	}
	docs, err := client.GetDocs(context.Background(), Context7DocsRequest{LibraryID: search.Results[0].ID, Query: "middleware auth", Type: "json"})
	if err != nil {
		t.Fatalf("GetDocs failed: %v", err)
	}
	formatted := FormatContext7DocsContent(docs, 500)
	if !strings.Contains(formatted, "Auth middleware") || !strings.Contains(formatted, "Middleware runs") {
		t.Fatalf("formatted docs = %s", formatted)
	}
	if urls := Context7DocsSourceURLs(docs); len(urls) != 2 {
		t.Fatalf("urls = %#v", urls)
	}
}

func TestContext7Client_HTTPStatusErrorRedactsSecret(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad key ctx7-secret", http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := NewContext7Client(Context7Endpoint{Name: "ctx", APIURL: ts.URL, APIKey: "ctx7-secret"})
	_, err := client.GetDocs(context.Background(), Context7DocsRequest{LibraryID: "/vercel/next.js", Query: "auth"})
	if err == nil {
		t.Fatal("expected error")
	}
	var statusErr HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T %v", err, err)
	}
	if statusErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", statusErr.StatusCode)
	}
	if strings.Contains(err.Error(), "ctx7-secret") {
		t.Fatalf("secret leaked in error: %v", err)
	}
}
