package router

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// builds n contacts whose ID is their index, so pages are distinguishable.
func makeContacts(n int) []database.ContactView {
	contacts := make([]database.ContactView, n)
	for i := range contacts {
		contacts[i].ID = strconv.Itoa(i)
	}
	return contacts
}

func TestPaginateContactsSplitsIntoFirstPageAndSpacers(t *testing.T) {
	tests := []struct {
		name         string
		n            int
		wantContacts int
		wantSpacers  []Spacer
	}{
		{"empty input", 0, 0, nil},
		{"exactly one page", pageSize, pageSize, nil},
		{"one row past a page", pageSize + 1, pageSize, []Spacer{
			{Query: "q", Page: 1, Count: 1},
		}},
		{"three and a half pages", 3*pageSize + pageSize/2, pageSize, []Spacer{
			{Query: "q", Page: 1, Count: pageSize},
			{Query: "q", Page: 2, Count: pageSize},
			{Query: "q", Page: 3, Count: pageSize / 2},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paginateContacts(makeContacts(tt.n), "q")
			assert.Len(t, got.Contacts, tt.wantContacts)
			assert.Equal(t, tt.wantSpacers, got.Spacers)
		})
	}
}

func TestPageSliceSelectsRequestedPage(t *testing.T) {
	contacts := makeContacts(2*pageSize + pageSize/2)

	tests := []struct {
		name      string
		page      int
		wantLen   int
		wantFirst string // ID of the first row on the page.
	}{
		{"page zero is the first page", 0, pageSize, "0"},
		{"negative page clamps to zero", -3, pageSize, "0"},
		{"last partial page", 2, pageSize / 2, strconv.Itoa(2 * pageSize)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pageSlice(contacts, tt.page)
			require.Len(t, got, tt.wantLen)
			assert.Equal(t, tt.wantFirst, got[0].ID)
		})
	}

	t.Run("page beyond range is nil", func(t *testing.T) {
		assert.Nil(t, pageSlice(contacts, 5))
	})

	t.Run("start exactly at len is empty", func(t *testing.T) {
		exact := makeContacts(pageSize)
		assert.Empty(t, pageSlice(exact, 1))
	})
}

func TestNewDavTokenSecretAndHash(t *testing.T) {
	secret, hash, err := newDavToken()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(secret, "bbook_"), "secret %q should carry the bbook_ prefix", secret)

	want := sha256.Sum256([]byte(secret))
	assert.Equal(t, want[:], hash, "hash should be the sha256 of the full secret string")

	secret2, _, err := newDavToken()
	require.NoError(t, err)
	assert.NotEqual(t, secret, secret2, "two tokens should not collide")
}

func TestStatusRecorderCapturesStatus(t *testing.T) {
	inner := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}
	rec.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rec.status)
	assert.Equal(t, http.StatusNotFound, inner.Code)
}

func TestRouterRoutes(t *testing.T) {
	prev := davBaseURL
	t.Cleanup(func() { davBaseURL = prev })

	r := Router(&config.Config{BaseURL: "https://bbook.example"})

	t.Run("ping is public", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/ping", nil))

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "pong!", rec.Body.String())
	})

	t.Run("index without session redirects to login", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil))

		require.Equal(t, http.StatusFound, rec.Code)
		assert.True(t, strings.HasPrefix(rec.Header().Get("Location"), "/auth/login?return_to="),
			"Location = %q", rec.Header().Get("Location"))
	})

	t.Run("carddav without basic auth is 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), "PROPFIND", "/api/dav/principal/", nil))

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "Basic")
	})
}
