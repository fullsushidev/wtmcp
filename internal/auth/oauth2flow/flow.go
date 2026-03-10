// Package oauth2flow implements the OAuth2 desktop authorization flow
// (browser consent + local callback server) for acquiring initial tokens.
//
// After tokens are acquired and saved, the core's OAuth2Provider handles
// automatic refresh. This package is used by CLI tools like bragctl for
// the one-time setup flow.
package oauth2flow

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
)

// Config holds the parameters for an OAuth2 desktop flow.
type Config struct {
	// ClientCredentialsFile is the path to the OAuth2 client credentials
	// JSON (downloaded from Google Cloud Console, etc.).
	ClientCredentialsFile string

	// TokenFile is where the acquired token will be saved.
	TokenFile string

	// Scopes are the OAuth2 scopes to request.
	Scopes []string

	// Port for the local callback server. 0 = auto-detect.
	Port int
}

// Run executes the OAuth2 desktop authorization flow:
//  1. Loads client credentials from the JSON file
//  2. Starts a local HTTP server for the OAuth2 callback
//  3. Opens the browser to the consent URL
//  4. Waits for the authorization code via callback
//  5. Exchanges the code for a token
//  6. Saves the token to disk
//
// Returns the acquired token or an error.
func Run(ctx context.Context, cfg Config) (*oauth2.Token, error) {
	oauthCfg, err := loadClientCredentials(cfg.ClientCredentialsFile, cfg.Scopes)
	if err != nil {
		return nil, fmt.Errorf("load client credentials: %w", err)
	}

	// Find available port for callback server
	port := cfg.Port
	if port == 0 {
		port, err = findAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("find available port: %w", err)
		}
	}
	oauthCfg.RedirectURL = fmt.Sprintf("http://localhost:%d/callback", port)

	// Channel to receive the authorization code
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Start local callback server
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errMsg := r.URL.Query().Get("error")
			if errMsg == "" {
				errMsg = "no authorization code received"
			}
			_, _ = fmt.Fprintf(w, "<html><body><h2>Authorization failed</h2><p>%s</p></body></html>", errMsg)
			errCh <- fmt.Errorf("authorization failed: %s", errMsg)
			return
		}
		_, _ = fmt.Fprint(w, "<html><body><h2>Authorization successful</h2><p>You can close this tab.</p></body></html>")
		codeCh <- code
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("localhost:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server: %w", err)
		}
	}()
	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	// Generate consent URL
	authURL := oauthCfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Open browser
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open browser: %w\n\nManually open this URL:\n%s", err, authURL)
	}

	// Wait for authorization code or error
	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Exchange code for token
	tok, err := oauthCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange code for token: %w", err)
	}

	// Save token
	if err := saveToken(cfg.TokenFile, tok); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	return tok, nil
}

// tokenJSON is the on-disk format for cached OAuth2 tokens.
// Compatible with the core's auth.OAuth2Provider token format
// and Google's Python oauth2client format.
type tokenJSON struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	Expiry       string `json:"expiry,omitempty"`
}

func saveToken(path string, tok *oauth2.Token) error {
	tj := tokenJSON{
		AccessToken:  tok.AccessToken,
		TokenType:    tok.TokenType,
		RefreshToken: tok.RefreshToken,
	}
	if !tok.Expiry.IsZero() {
		tj.Expiry = tok.Expiry.Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(tj, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create token dir: %w", err)
	}

	return os.WriteFile(path, data, 0o600)
}

// clientCredentialsJSON represents the OAuth2 client credentials file
// (downloaded from Google Cloud Console).
type clientCredentialsJSON struct {
	Installed *clientData `json:"installed"`
	Web       *clientData `json:"web"`
}

type clientData struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

func loadClientCredentials(path string, scopes []string) (*oauth2.Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // credential file path from user
	if err != nil {
		return nil, err
	}

	var creds clientCredentialsJSON
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	cd := creds.Installed
	if cd == nil {
		cd = creds.Web
	}
	if cd == nil {
		return nil, fmt.Errorf("no 'installed' or 'web' credentials found in %s", path)
	}

	return &oauth2.Config{
		ClientID:     cd.ClientID,
		ClientSecret: cd.ClientSecret,
		Scopes:       scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  cd.AuthURI,
			TokenURL: cd.TokenURI,
		},
	}, nil
}

func findAvailablePort() (int, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}
