package auth

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	cookieState    = "db_oauth_state"
	cookieSession  = "db_session"
	cookieActLogin = "db_act_login" // set after a login activity event for this SSO session
	stateTTL       = 10 * time.Minute
	// Long enough for multi-hour interviews; tokens are refreshed underneath.
	sessionCookieTTL = 24 * time.Hour
	// Refresh a minute before access-token expiry when possible.
	refreshSkew = time.Minute
	// Browsers silently drop Set-Cookie values near/over ~4KiB.
	maxSessionCookieBytes = 3500
)

type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AppPublicURL string
	CAFile       string
	Enabled      bool
}

func ConfigFromEnv() Config {
	issuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER"))
	if issuer == "" {
		base := strings.TrimRight(os.Getenv("KEYCLOAK_URL"), "/")
		realm := os.Getenv("KEYCLOAK_REALM")
		if realm == "" {
			realm = "dasmlab"
		}
		if base != "" {
			issuer = base + "/realms/" + realm
		}
	}
	appURL := strings.TrimRight(os.Getenv("APP_PUBLIC_URL"), "/")
	redirect := strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URI"))
	if redirect == "" && appURL != "" {
		redirect = appURL + "/api/v1/auth/callback"
	}
	cfg := Config{
		Issuer:       issuer,
		ClientID:     envOr("OIDC_CLIENT_ID", "dasm-burner"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		RedirectURL:  redirect,
		AppPublicURL: appURL,
		CAFile:       strings.TrimSpace(os.Getenv("OIDC_CA_FILE")),
	}
	cfg.Enabled = cfg.Issuer != "" && cfg.ClientID != "" && cfg.ClientSecret != "" && cfg.RedirectURL != ""
	return cfg
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func httpClientForOIDC(caFile string) (*http.Client, error) {
	if caFile == "" {
		return http.DefaultClient, nil
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		// Preview mounts mark the OIDC CA ConfigMap optional; fall back to system roots
		// so a missing bootstrap does not brick the process after the pod finally starts.
		fmt.Fprintf(os.Stderr, "dasm-burner: OIDC CA file %q unavailable (%v); using system roots\n", caFile, err)
		return http.DefaultClient, nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates found in OIDC CA file %s", caFile)
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}

type User struct {
	Sub               string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username"`
	Email             string   `json:"email"`
	Name              string   `json:"name"`
	Roles             []string `json:"roles"`
	IsAdmin           bool     `json:"is_admin"`
}

// Client role name that grants full admin API / UI access.
const RoleAdmin = "admin"

type ctxKey int

const userContextKey ctxKey = 1

// UserFromContext returns the admin-authenticated user set by AdminMiddleware.
func UserFromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userContextKey).(*User)
	return u, ok && u != nil
}

type Service struct {
	cfg      Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
	http     *http.Client

	mu     sync.Mutex
	states map[string]time.Time

	// OnLogin is invoked once per SSO session after a successful login
	// (OIDC callback and/or first /auth/me). Optional.
	OnLogin func(user *User)
}

func New(ctx context.Context, cfg Config) (*Service, error) {
	if !cfg.Enabled {
		return &Service{cfg: cfg, states: map[string]time.Time{}, http: http.DefaultClient}, nil
	}
	httpClient, err := httpClientForOIDC(cfg.CAFile)
	if err != nil {
		return nil, err
	}
	ctx = oidc.ClientContext(ctx, httpClient)
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}
	s := &Service{
		cfg:      cfg,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		http:     httpClient,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		states: map[string]time.Time{},
	}
	return s, nil
}

func (s *Service) Enabled() bool { return s != nil && s.cfg.Enabled }

func (s *Service) ConfigInfo() map[string]any {
	return map[string]any{
		"enabled":        s.Enabled(),
		"issuer":         s.cfg.Issuer,
		"client_id":      s.cfg.ClientID,
		"redirect_uri":   s.cfg.RedirectURL,
		"app_public_url": s.cfg.AppPublicURL,
	}
}

func (s *Service) oauthCtx(ctx context.Context) context.Context {
	if s.http == nil {
		return ctx
	}
	return context.WithValue(ctx, oauth2.HTTPClient, s.http)
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	if !s.Enabled() {
		http.Error(w, "OIDC not configured", http.StatusServiceUnavailable)
		return
	}
	state, err := randomString(24)
	if err != nil {
		http.Error(w, "failed to create state", http.StatusInternalServerError)
		return
	}
	s.putState(state)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieState,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(stateTTL.Seconds()),
	})
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, oauth2.AccessTypeOffline), http.StatusFound)
}

func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	if !s.Enabled() {
		http.Error(w, "OIDC not configured", http.StatusServiceUnavailable)
		return
	}
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		http.Error(w, "oidc error: "+errMsg+" — "+r.URL.Query().Get("error_description"), http.StatusBadRequest)
		return
	}
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie(cookieState)
	if err != nil || cookie.Value == "" || cookie.Value != state || !s.takeState(state) {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieState, Value: "", Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	token, err := s.oauth.Exchange(s.oauthCtx(r.Context()), code)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok || rawID == "" {
		http.Error(w, "missing id_token", http.StatusBadRequest)
		return
	}
	if _, err := s.verifier.Verify(r.Context(), rawID); err != nil {
		http.Error(w, "id_token verify failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	if err := s.writeSessionCookie(w, r, sessionFromToken(token, rawID)); err != nil {
		http.Error(w, "session encode failed", http.StatusInternalServerError)
		return
	}

	if user, uerr := s.userFromTokens(r.Context(), token.AccessToken, rawID); uerr == nil {
		s.emitLoginOnce(w, r, user)
	}

	dest := s.cfg.AppPublicURL
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest+"/", http.StatusFound)
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	var idToken string
	if c, err := r.Cookie(cookieSession); err == nil {
		if sess, err := decodeSession(c.Value); err == nil {
			idToken = sess.IDToken
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieSession, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: cookieActLogin, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})

	if !s.Enabled() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	endSession := s.cfg.Issuer + "/protocol/openid-connect/logout"
	u, _ := url.Parse(endSession)
	q := u.Query()
	if idToken != "" {
		q.Set("id_token_hint", idToken)
	}
	postLogout := s.cfg.AppPublicURL
	if postLogout == "" {
		postLogout = "/"
	}
	q.Set("post_logout_redirect_uri", postLogout+"/")
	q.Set("client_id", s.cfg.ClientID)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	user, err := s.Authenticate(w, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	// Session-start backup if callback did not emit (e.g. existing cookie / refresh).
	s.emitLoginOnce(w, r, user)
	writeJSON(w, http.StatusOK, user)
}

// emitLoginOnce records a login activity event at most once per SSO browser session.
func (s *Service) emitLoginOnce(w http.ResponseWriter, r *http.Request, user *User) {
	if s.OnLogin == nil || user == nil {
		return
	}
	if c, err := r.Cookie(cookieActLogin); err == nil && c.Value == "1" {
		return
	}
	s.OnLogin(user)
	if w != nil {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieActLogin,
			Value:    "1",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isHTTPS(r),
			MaxAge:   int(sessionCookieTTL.Seconds()),
		})
	}
}

// KeepAlive refreshes the SSO session cookie (access token via refresh_token).
func (s *Service) KeepAlive(w http.ResponseWriter, r *http.Request) {
	user, err := s.Authenticate(w, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"is_admin": user.IsAdmin,
		"roles":    user.Roles,
	})
}

// Authenticate resolves the user and silently refreshes tokens when near expiry.
func (s *Service) Authenticate(w http.ResponseWriter, r *http.Request) (*User, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("oidc disabled")
	}
	if bearer := bearerToken(r); bearer != "" {
		return s.userFromTokens(r.Context(), bearer, "")
	}
	c, err := r.Cookie(cookieSession)
	if err != nil || c.Value == "" {
		return nil, fmt.Errorf("no token")
	}
	sess, err := decodeSession(c.Value)
	if err != nil {
		return nil, err
	}

	needRefresh := sess.AccessToken == "" || sess.RefreshToken != "" && (sess.Expiry == 0 ||
		time.Until(time.Unix(sess.Expiry, 0)) < refreshSkew)
	if needRefresh && sess.RefreshToken != "" {
		if refreshed, rerr := s.refreshSession(r.Context(), sess); rerr == nil {
			sess = refreshed
			_ = s.writeSessionCookie(w, r, sess)
		} else if sess.AccessToken == "" {
			return nil, rerr
		}
	}

	user, err := s.userFromTokens(r.Context(), sess.AccessToken, sess.IDToken)
	if err != nil && sess.RefreshToken != "" {
		refreshed, rerr := s.refreshSession(r.Context(), sess)
		if rerr != nil {
			return nil, err
		}
		sess = refreshed
		_ = s.writeSessionCookie(w, r, sess)
		user, err = s.userFromTokens(r.Context(), sess.AccessToken, sess.IDToken)
	}
	if err != nil {
		return nil, err
	}
	// Slide cookie lifetime while the interview is active.
	_ = s.writeSessionCookie(w, r, sess)
	return user, nil
}

func (s *Service) UserFromRequest(r *http.Request) (*User, error) {
	return s.Authenticate(nil, r)
}

func (s *Service) userFromTokens(ctx context.Context, accessTok, idTok string) (*User, error) {
	if accessTok == "" && idTok == "" {
		return nil, fmt.Errorf("no token")
	}

	var user *User
	var lastErr error
	if accessTok != "" {
		u, err := s.userFromAccessToken(ctx, accessTok)
		if err == nil {
			user = u
		} else {
			lastErr = err
		}
	}
	if user == nil && idTok != "" {
		parsed, err := s.verifier.Verify(ctx, idTok)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}
		var claims User
		if err := parsed.Claims(&claims); err != nil {
			return nil, err
		}
		user = &claims
	}
	if user == nil {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no token")
	}

	roles := clientRolesFromJWT(accessTok, s.cfg.ClientID)
	if len(roles) == 0 {
		roles = clientRolesFromJWT(idTok, s.cfg.ClientID)
	}
	user.Roles = roles
	user.IsAdmin = hasRole(roles, RoleAdmin)
	return user, nil
}

func sessionFromToken(token *oauth2.Token, idToken string) sessionPayload {
	if idToken == "" {
		if v, ok := token.Extra("id_token").(string); ok {
			idToken = v
		}
	}
	return sessionPayload{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      idToken,
		Expiry:       token.Expiry.Unix(),
	}
}

func (s *Service) refreshSession(ctx context.Context, sess sessionPayload) (sessionPayload, error) {
	if sess.RefreshToken == "" {
		return sess, fmt.Errorf("no refresh token")
	}
	src := s.oauth.TokenSource(s.oauthCtx(ctx), &oauth2.Token{
		AccessToken:  sess.AccessToken,
		RefreshToken: sess.RefreshToken,
		Expiry:       time.Unix(sess.Expiry, 0),
	})
	tok, err := src.Token()
	if err != nil {
		return sess, err
	}
	next := sessionFromToken(tok, "")
	if next.IDToken == "" {
		next.IDToken = sess.IDToken
	}
	if next.RefreshToken == "" {
		next.RefreshToken = sess.RefreshToken
	}
	return next, nil
}

func (s *Service) writeSessionCookie(w http.ResponseWriter, r *http.Request, sess sessionPayload) error {
	if w == nil {
		return nil
	}
	val, err := encodeSession(fitSessionCookie(sess))
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieSession,
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionCookieTTL.Seconds()),
	})
	return nil
}

// fitSessionCookie shrinks the payload so Set-Cookie survives browser size limits.
// Always prefer refresh_token — access/id JWTs with realm roles routinely push
// the combined cookie over ~4KiB, which browsers silently drop (login loop).
func fitSessionCookie(sess sessionPayload) sessionPayload {
	slim := sessionPayload{
		RefreshToken: sess.RefreshToken,
		Expiry:       sess.Expiry,
	}
	if slim.RefreshToken == "" {
		// Legacy / bearer-less edge: keep whatever we have if it fits.
		if val, err := encodeSession(sess); err == nil && len(val) <= maxSessionCookieBytes {
			return sess
		}
		return sessionPayload{AccessToken: sess.AccessToken, Expiry: sess.Expiry}
	}
	// Include id_token only when it still fits (used for logout hint).
	withID := sessionPayload{
		RefreshToken: sess.RefreshToken,
		IDToken:      sess.IDToken,
		Expiry:       sess.Expiry,
	}
	if sess.IDToken != "" {
		if val, err := encodeSession(withID); err == nil && len(val) <= maxSessionCookieBytes {
			return withID
		}
	}
	return slim
}

func (s *Service) userFromAccessToken(ctx context.Context, raw string) (*User, error) {
	ui, err := s.provider.UserInfo(s.oauthCtx(ctx), oauth2.StaticTokenSource(&oauth2.Token{AccessToken: raw}))
	if err != nil {
		return nil, err
	}
	var claims User
	if err := ui.Claims(&claims); err != nil {
		return nil, err
	}
	if claims.Sub == "" {
		claims.Sub = ui.Subject
	}
	return &claims, nil
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return s.AdminMiddleware(next)
}

// GuestUser is the unauthenticated viewer (no Keycloak session).
func GuestUser() *User {
	return &User{PreferredUsername: "guest", Name: "Guest", Roles: []string{"guest"}, IsAdmin: false}
}

// GuestMiddleware allows unauthenticated viewers. A Keycloak session is attached when
// present (including users without the admin role). Local/dev (OIDC off) is full admin.
func (s *Service) GuestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Enabled() {
			local := &User{PreferredUsername: "local", Name: "Local Dev", IsAdmin: true}
			ctx := context.WithValue(r.Context(), userContextKey, local)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		user, err := s.Authenticate(w, r)
		if err != nil {
			user = GuestUser()
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminMiddleware requires a Keycloak session with the dasm-burner client role "admin".
// When OIDC is disabled it allows all requests (local/dev mode).
func (s *Service) AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.Enabled() {
			local := &User{PreferredUsername: "local", Name: "Local Dev", IsAdmin: true}
			ctx := context.WithValue(r.Context(), userContextKey, local)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		user, err := s.Authenticate(w, r)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if !user.IsAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":  "forbidden",
				"detail": "requires dasm-burner client role: admin",
			})
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ActivityViewers returns preferred_usernames allowed to GET /activity.
// Default: dasm. Override with ACTIVITY_VIEWERS=comma,separated,list.
func ActivityViewers() []string {
	raw := strings.TrimSpace(os.Getenv("ACTIVITY_VIEWERS"))
	if raw == "" {
		return []string{"dasm"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"dasm"}
	}
	return out
}

// CanViewActivity reports whether preferred_username is on the activity viewer allowlist.
// When OIDC is disabled, local/dev may view (for lab testing).
func (s *Service) CanViewActivity(user *User) bool {
	if user == nil {
		return false
	}
	if !s.Enabled() {
		return true
	}
	uname := strings.TrimSpace(user.PreferredUsername)
	for _, v := range ActivityViewers() {
		if uname == v {
			return true
		}
	}
	return false
}

// ActivityViewerGate is for use inside AdminMiddleware: forbid unless viewer allowlist.
func (s *Service) ActivityViewerGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := UserFromContext(r.Context())
		if !ok || !s.CanViewActivity(user) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":  "forbidden",
				"detail": "activity log is restricted",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IsAdmin reports whether the request has the dasm-burner "admin" client role.
// When OIDC is disabled, returns true. Pass w so tokens can be refreshed.
func (s *Service) IsAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !s.Enabled() {
		return true
	}
	user, err := s.Authenticate(w, r)
	return err == nil && user.IsAdmin
}

// clientRolesFromJWT reads resource_access.<clientID>.roles from a JWT payload.
func clientRolesFromJWT(raw, clientID string) []string {
	if raw == "" || clientID == "" {
		return nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some issuers pad; try standard encoding.
		payload, err = base64.RawStdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var claims struct {
		ResourceAccess map[string]struct {
			Roles []string `json:"roles"`
		} `json:"resource_access"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	ra, ok := claims.ResourceAccess[clientID]
	if !ok {
		return nil
	}
	return ra.Roles
}

func hasRole(roles []string, want string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, want) {
			return true
		}
	}
	return false
}

func (s *Service) putState(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, exp := range s.states {
		if now.After(exp) {
			delete(s.states, k)
		}
	}
	s.states[state] = now.Add(stateTTL)
}

func (s *Service) takeState(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.states[state]
	if !ok {
		return false
	}
	delete(s.states, state)
	return time.Now().Before(exp)
}

type sessionPayload struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token"`
	Expiry       int64  `json:"expiry"`
}

func encodeSession(s sessionPayload) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeSession(v string) (sessionPayload, error) {
	var s sessionPayload
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
