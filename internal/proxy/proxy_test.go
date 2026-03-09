package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/auth"
	"gitlab.cee.redhat.com/bragctl/what-the-mcp/internal/protocol"
)

func TestExecuteGET(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/test" {
			t.Errorf("path = %q, want /api/test", r.URL.Path)
		}
		if r.URL.Query().Get("foo") != "bar" {
			t.Errorf("query foo = %q, want bar", r.URL.Query().Get("foo"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:     "req-1",
		Type:   protocol.TypeHTTPRequest,
		Method: "GET",
		Path:   "/api/test",
		Query:  map[string]any{"foo": "bar"},
	})

	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if string(resp.Body) != `{"ok":true}` {
		t.Errorf("body = %s", resp.Body)
	}
}

func TestExecutePOST(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":true}`))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:      "req-2",
		Type:    protocol.TypeHTTPRequest,
		Method:  "POST",
		Path:    "/items",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    json.RawMessage(`{"name":"item1"}`),
	})

	if resp.Status != 200 {
		t.Errorf("status = %d", resp.Status)
	}
}

func TestExecuteWithAuth(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("Authorization = %q, want 'Bearer test-token'", authHeader)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{
		BaseURL:  srv.URL,
		Provider: auth.NewBearerProvider("test-token", "", ""),
	})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:     "req-3",
		Type:   protocol.TypeHTTPRequest,
		Method: "GET",
		Path:   "/secure",
	})

	if resp.Status != 200 {
		t.Errorf("status = %d, error = %v", resp.Status, resp.Error)
	}
}

func TestExecuteQueryArrays(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query()["field"]
		if len(fields) != 2 || fields[0] != "summary" || fields[1] != "status" {
			t.Errorf("field = %v, want [summary, status]", fields)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:     "req-4",
		Type:   protocol.TypeHTTPRequest,
		Method: "GET",
		Path:   "/search",
		Query:  map[string]any{"field": []any{"summary", "status"}},
	})

	if resp.Status != 200 {
		t.Errorf("status = %d, error = %v", resp.Status, resp.Error)
	}
}

func TestExecuteResponseBodyLimit(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write a body larger than the limit
		data := make([]byte, 1024)
		for i := range data {
			data[i] = 'x'
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	// Set a tiny max body size
	p := New(srv.Client(), 100)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:     "req-5",
		Type:   protocol.TypeHTTPRequest,
		Method: "GET",
		Path:   "/big",
	})

	if resp.Error == nil {
		t.Error("expected error for oversized response")
	}
	if resp.Error != nil && resp.Error.Code != "response_too_large" {
		t.Errorf("error code = %q, want response_too_large", resp.Error.Code)
	}
}

func TestExecuteNonJSONResponse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:     "req-6",
		Type:   protocol.TypeHTTPRequest,
		Method: "GET",
		Path:   "/text",
	})

	if resp.Status != 200 {
		t.Errorf("status = %d", resp.Status)
	}
	// Non-JSON is returned as a quoted string
	if string(resp.Body) != `"hello world"` {
		t.Errorf("body = %s, want %q", resp.Body, `"hello world"`)
	}
}

func TestExecuteUnknownPlugin(t *testing.T) {
	p := New(nil, 10*1024*1024)

	resp := p.Execute(context.Background(), "nonexistent", protocol.Message{
		ID: "req-7", Type: protocol.TypeHTTPRequest, Method: "GET", Path: "/",
	})

	if resp.Error == nil || resp.Error.Code != "no_config" {
		t.Errorf("expected no_config error, got %v", resp.Error)
	}
}

func TestIsDomainAllowed(t *testing.T) {
	p := New(nil, 10*1024*1024)

	tests := []struct {
		name    string
		pa      *PluginAuth
		rawURL  string
		allowed bool
	}{
		{
			name:    "same domain",
			pa:      &PluginAuth{BaseURL: "https://api.example.com"},
			rawURL:  "https://api.example.com/other",
			allowed: true,
		},
		{
			name:    "case insensitive",
			pa:      &PluginAuth{BaseURL: "https://api.example.com"},
			rawURL:  "https://API.EXAMPLE.COM/other",
			allowed: true,
		},
		{
			name:    "allowed domain",
			pa:      &PluginAuth{BaseURL: "https://api.example.com", AllowedDomains: []string{"cdn.example.com"}},
			rawURL:  "https://cdn.example.com/file",
			allowed: true,
		},
		{
			name:    "different domain",
			pa:      &PluginAuth{BaseURL: "https://api.example.com"},
			rawURL:  "https://evil.com/steal",
			allowed: false,
		},
		{
			name:    "http with auth rejects",
			pa:      &PluginAuth{BaseURL: "https://api.example.com", Provider: auth.NewBearerProvider("tok", "", "")},
			rawURL:  "http://api.example.com/insecure",
			allowed: false,
		},
		{
			name:    "userinfo rejects",
			pa:      &PluginAuth{BaseURL: "https://api.example.com"},
			rawURL:  "https://evil@api.example.com/path",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.isDomainAllowed("test", tt.pa, tt.rawURL)
			if got != tt.allowed {
				t.Errorf("isDomainAllowed = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestExecuteFullURLOverride(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"override":true}`))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID:     "req-8",
		Type:   protocol.TypeHTTPRequest,
		Method: "GET",
		URL:    srv.URL + "/full-override",
		Path:   "/ignored",
	})

	if resp.Status != 200 {
		t.Errorf("status = %d, error = %v", resp.Status, resp.Error)
	}
}

func TestResponseHeaders(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "test-value")
		w.Header().Set("Content-Disposition", `attachment; filename="report.pdf"`)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID: "req-headers", Type: protocol.TypeHTTPRequest, Method: "GET", Path: "/",
	})

	if resp.Headers == nil {
		t.Fatal("expected response headers")
	}
	if resp.Headers["X-Custom"] != "test-value" {
		t.Errorf("X-Custom = %q", resp.Headers["X-Custom"])
	}
	if resp.Headers["Content-Disposition"] != `attachment; filename="report.pdf"` {
		t.Errorf("Content-Disposition = %q", resp.Headers["Content-Disposition"])
	}
}

func TestBinaryResponse(t *testing.T) {
	// PNG magic bytes
	pngData := []byte{0x89, 0x50, 0x4e, 0x47}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngData)
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID: "req-bin", Type: protocol.TypeHTTPRequest, Method: "GET", Path: "/image.png",
	})

	if resp.Status != 200 {
		t.Fatalf("status = %d, error = %v", resp.Status, resp.Error)
	}
	if resp.BodyEncoding != "base64" {
		t.Errorf("BodyEncoding = %q, want base64", resp.BodyEncoding)
	}

	// Body should be a JSON string containing the base64 data
	var b64str string
	if err := json.Unmarshal(resp.Body, &b64str); err != nil {
		t.Fatalf("unmarshal base64 string: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64str)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if string(decoded) != string(pngData) {
		t.Errorf("decoded = %x, want %x", decoded, pngData)
	}
}

func TestTextResponse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	p := New(srv.Client(), 10*1024*1024)
	p.RegisterPlugin("test", &PluginAuth{BaseURL: srv.URL})

	resp := p.Execute(context.Background(), "test", protocol.Message{
		ID: "req-text", Type: protocol.TypeHTTPRequest, Method: "GET", Path: "/text",
	})

	if resp.Status != 200 {
		t.Fatalf("status = %d, error = %v", resp.Status, resp.Error)
	}
	if resp.BodyEncoding != "" {
		t.Errorf("BodyEncoding = %q, want empty for text", resp.BodyEncoding)
	}

	var text string
	if err := json.Unmarshal(resp.Body, &text); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q", text)
	}
}
