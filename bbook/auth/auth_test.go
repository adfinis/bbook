package auth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const (
	testSub     = "alice"
	signPayload = `{"sub":"alice"}`
	anonURI     = "/books?page=2&q=go"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	setTestSecret(t, testSecret)
	for _, payload := range [][]byte{[]byte(signPayload), {}} {
		got, err := verify(sign(payload))
		require.NoError(t, err)
		assert.Equal(t, payload, got)
	}
}

func TestVerifyRejectsInvalidInputs(t *testing.T) {
	setTestSecret(t, testSecret)
	valid := sign([]byte(signPayload))
	parts := strings.SplitN(valid, ".", 2)
	forgedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"mallory"}`))
	tests := []struct{ name, value string }{
		{"tampered payload", forgedPayload + "." + parts[1]},
		{"tampered signature", parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("not-the-mac"))},
		{"missing separator", parts[0]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := verify(tt.value)
			assert.Error(t, err)
		})
	}
}

func TestVerifyRejectsDifferentSecret(t *testing.T) {
	setTestSecret(t, testSecret)
	signed := sign([]byte(signPayload))
	setTestSecret(t, otherTestSecret)
	_, err := verify(signed)
	assert.Error(t, err)
}

func TestInGroup(t *testing.T) {
	tests := []struct {
		name   string
		groups []string
		want   string
		expect bool
	}{
		{"exact match", []string{"users", "admins"}, "admins", true},
		{"leading slash on group", []string{"/admins"}, "admins", true},
		{"leading slash on want", []string{"admins"}, "/admins", true},
		{"non-membership", []string{"users"}, "admins", false},
		{"empty groups", nil, "admins", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, inGroup(tt.groups, tt.want))
		})
	}
}

// requestWithSessionCookie builds a request carrying a validly signed session cookie.
func requestWithSessionCookie(t *testing.T, target string, s sessionClaims) *http.Request {
	t.Helper()
	rec := httptest.NewRecorder()
	require.NoError(t, setSignedCookie(rec, sessionCookieName, s, sessionTTL))
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(rec.Result().Cookies()[0])
	return req
}

func TestSignedCookieRoundTrip(t *testing.T) {
	setTestSecret(t, testSecret)
	rec := httptest.NewRecorder()
	in := sessionClaims{Sub: testSub, Exp: 42}
	require.NoError(t, setSignedCookie(rec, sessionCookieName, in, sessionTTL))

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	c := cookies[0]
	assert.Equal(t, sessionCookieName, c.Name)
	assert.True(t, c.HttpOnly)
	assert.True(t, c.Secure)
	assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
	assert.Equal(t, int(sessionTTL.Seconds()), c.MaxAge)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	var out sessionClaims
	require.NoError(t, readSignedCookie(req, sessionCookieName, &out))
	assert.Equal(t, in, out)
}

func TestCurrentUserSub(t *testing.T) {
	setTestSecret(t, testSecret)
	t.Run("valid session", func(t *testing.T) {
		req := requestWithSessionCookie(t, "/", sessionClaims{Sub: testSub, Exp: time.Now().Add(time.Hour).Unix()})
		assert.Equal(t, testSub, CurrentUserSub(req))
	})
	t.Run("expired session", func(t *testing.T) {
		req := requestWithSessionCookie(t, "/", sessionClaims{Sub: testSub, Exp: time.Now().Add(-time.Hour).Unix()})
		assert.Empty(t, CurrentUserSub(req))
	})
	t.Run("garbage cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "garbage"})
		assert.Empty(t, CurrentUserSub(req))
	})
}

func TestMiddlewareAllowsValidSession(t *testing.T) {
	setTestSecret(t, testSecret)
	called := false
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := requestWithSessionCookie(t, anonURI, sessionClaims{Sub: testSub, Exp: time.Now().Add(time.Hour).Unix()})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.True(t, called)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestMiddlewareRedirectsAnonymousToLogin(t *testing.T) {
	setTestSecret(t, testSecret)
	h := Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next handler must not be reached")
	}))
	req := httptest.NewRequest(http.MethodGet, anonURI, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/auth/login?return_to="+url.QueryEscape(anonURI), rec.Header().Get("Location"))
}

// A 302 on an htmx request would make htmx swap in the login page HTML instead of navigating; expired sessions must get 200 + HX-Redirect.
func TestMiddlewareHTMXRedirectHeader(t *testing.T) {
	setTestSecret(t, testSecret)
	h := Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next handler must not be reached")
	}))
	req := httptest.NewRequest(http.MethodGet, anonURI, nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/auth/login?return_to="+url.QueryEscape(anonURI), rec.Header().Get("HX-Redirect"))
}

// setTestOAuthCfg installs a stub oauth2 config pointing at a fake IdP, restoring the previous one after the test.
func setTestOAuthCfg(t *testing.T) {
	t.Helper()
	old := oauthCfg
	t.Cleanup(func() { oauthCfg = old })
	oauthCfg = &oauth2.Config{
		ClientID:    "test-client",
		Endpoint:    oauth2.Endpoint{AuthURL: "https://idp.example/auth"},
		RedirectURL: "https://bbook.example/auth/callback",
	}
}

func TestLoginHandlerRejectsBadReturnTo(t *testing.T) {
	setTestSecret(t, testSecret)
	setTestOAuthCfg(t)
	tests := []struct{ name, returnTo string }{
		{"absolute url", "https://evil.example"},
		{"scheme-relative", "//evil.example"},
		{"backslash", `/foo\bar`},
		{"not rooted", "foo/bar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/login?return_to="+url.QueryEscape(tt.returnTo), nil)
			rec := httptest.NewRecorder()
			LoginHandler(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestLoginHandlerRedirectsToIdP(t *testing.T) {
	setTestSecret(t, testSecret)
	setTestOAuthCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/login?return_to="+url.QueryEscape("/path?x=1"), nil)
	rec := httptest.NewRecorder()
	LoginHandler(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)

	loc, err := url.Parse(rec.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "idp.example", loc.Host)
	assert.Equal(t, "/auth", loc.Path)
	q := loc.Query()
	assert.NotEmpty(t, q.Get("state"))
	assert.NotEmpty(t, q.Get("nonce"))
	assert.Equal(t, "S256", q.Get("code_challenge_method"))
	assert.NotEmpty(t, q.Get("code_challenge"))

	// The flow cookie must be validly signed and carry the values embedded in the redirect.
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, flowCookieName, cookies[0].Name)
	cookieReq := httptest.NewRequest(http.MethodGet, "/", nil)
	cookieReq.AddCookie(cookies[0])
	var flow flowClaims
	require.NoError(t, readSignedCookie(cookieReq, flowCookieName, &flow))
	assert.Equal(t, q.Get("state"), flow.State)
	assert.Equal(t, q.Get("nonce"), flow.Nonce)
	assert.Equal(t, "/path?x=1", flow.ReturnTo)
	assert.Equal(t, q.Get("code_challenge"), oauth2.S256ChallengeFromVerifier(flow.Verifier))
	assert.Greater(t, flow.Exp, time.Now().Unix())
}

// flowCookie returns a validly signed flow cookie for the given claims.
func flowCookie(t *testing.T, flow flowClaims) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	require.NoError(t, setSignedCookie(rec, flowCookieName, flow, flowTTL))
	return rec.Result().Cookies()[0]
}

func TestCallbackHandlerRejectsMissingFlowCookie(t *testing.T) {
	setTestSecret(t, testSecret)
	rec := httptest.NewRecorder()
	CallbackHandler(rec, httptest.NewRequest(http.MethodGet, "/auth/callback?state=x&code=y", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCallbackHandlerRejectsExpiredFlow(t *testing.T) {
	setTestSecret(t, testSecret)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=x&code=y", nil)
	req.AddCookie(flowCookie(t, flowClaims{State: "x", Exp: time.Now().Add(-time.Minute).Unix()}))
	rec := httptest.NewRecorder()
	CallbackHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCallbackHandlerRejectsStateMismatch(t *testing.T) {
	setTestSecret(t, testSecret)
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=wrong&code=y", nil)
	req.AddCookie(flowCookie(t, flowClaims{State: "right", Exp: time.Now().Add(time.Minute).Unix()}))
	rec := httptest.NewRecorder()
	CallbackHandler(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTokenInRequiredGroupEmptyGroupAllowsAll(t *testing.T) {
	old := requiredGroup
	t.Cleanup(func() { requiredGroup = old })
	requiredGroup = ""
	ok, err := tokenInRequiredGroup(nil)
	require.NoError(t, err)
	assert.True(t, ok)
}
