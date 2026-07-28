package zoho

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.adfinis.com/int-infrastructure/bbook/bbook/config"
)

// snapshots the package-level client and restores it after the test.
func saveClient(t *testing.T) {
	t.Helper()
	orig := client
	t.Cleanup(func() { client = orig })
}

// points the package client at a fake CRM server with a valid cached token.
func setCRMClient(t *testing.T, ts *httptest.Server) {
	t.Helper()
	saveClient(t)
	client = zohoHTTPClient{
		httpClient:  ts.Client(),
		cfg:         &config.Config{ZohoSyncEnabled: true, ZohoBaseURL: ts.URL},
		accessToken: "cached-token",
		expiry:      time.Now().Add(time.Hour),
	}
}

func TestSanitizeField(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain string untouched", "hello world", "hello world"},
		{"control chars become spaces", "a\nb\tc\x00d", "a b c d"},
		{"whitespace collapses and trims", "  a  \t\n  b  ", "a b"},
		{"invalid utf-8 dropped", "caf\xffe", "cafe"},
		{"only whitespace", " \t\n ", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizeField(tc.in))
		})
	}
}

func TestSanitizeRawJSON(t *testing.T) {
	t.Run("removes nul escapes", func(t *testing.T) {
		got := sanitizeRawJSON(json.RawMessage(`{"a":"b\u0000c"}`))
		assert.Equal(t, `{"a":"bc"}`, string(got))
		assert.True(t, json.Valid(got))
	})
	t.Run("leaves normal json intact", func(t *testing.T) {
		in := `{"a":"b","n":1,"u":"café"}`
		assert.Equal(t, in, string(sanitizeRawJSON(json.RawMessage(in))))
	})
}

func TestBuildJSONFieldsSkipsAndStripsTags(t *testing.T) {
	type sample struct {
		ID       string `json:"id"`
		A        string `json:"a,omitempty"`
		B        string `json:"-"`
		Untagged string
		D        string `json:"d"`
	}
	assert.Equal(t, "a,d", buildJSONFields(reflect.TypeFor[sample]()))
}

func TestRawZohoContactUnmarshalCapturesRaw(t *testing.T) {
	data := []byte(`{"id":"42","First_Name":"Anna","Contact_Status":"Active"}`)
	var rc rawZohoContact
	require.NoError(t, json.Unmarshal(data, &rc))
	assert.Equal(t, "42", rc.ID)
	assert.Equal(t, "Anna", rc.FirstName, "fields must survive the custom unmarshaler")
	assert.Equal(t, data, []byte(rc.Raw), "Raw must hold the original bytes")
}

func TestToContactMapsAndSanitizes(t *testing.T) {
	mod := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	rc := rawZohoContact{
		ID:        " 42\n",
		FirstName: "Anna\tMarie",
		LastName:  " Muster ",
		Email:     "a@b.ch\x00",
		Phone:     "+41 44 123 45 67",
		Mobile:    "+41 79 987 65 43",
		AccountName: &struct {
			Name string `json:"name"`
			Id   string `json:"id"`
		}{Name: "Adfinis\nAG", Id: "9"},
		MailingStreet: "Bahnhofstrasse  1",
		MailingCity:   "Zürich",
		MailingZip:    "8001",
		Status:        "Active",
		ModifiedTime:  mod,
		Raw:           json.RawMessage(`{"Last_Name":"Mu\u0000ster"}`),
	}

	c := rc.toContact()
	assert.Equal(t, "42", c.ZohoID)
	assert.Equal(t, "Anna Marie", c.FirstName)
	assert.Equal(t, "Muster", c.LastName)
	assert.Equal(t, "Adfinis AG", c.Organization)
	assert.Equal(t, "a@b.ch", c.Email)
	assert.Equal(t, "+41441234567", c.Phone, "spaces removed from phone before sanitize")
	assert.Equal(t, "+41799876543", c.Mobile)
	assert.Equal(t, "Bahnhofstrasse 1", c.Street)
	assert.Equal(t, "Zürich", c.City)
	assert.Equal(t, "8001", c.PostalCode)
	assert.Equal(t, "Active", c.Status)
	assert.Equal(t, mod, c.ModifiedTime)
	assert.Equal(t, `{"Last_Name":"Muster"}`, string(c.Raw), "raw json nul escape stripped")
}

func TestTokenRefreshesAndCaches(t *testing.T) {
	saveClient(t)
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "/oauth/v2/token", r.URL.Path)
		// assert, not require: FailNow must not run off the test goroutine.
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "refresh_token", r.PostForm.Get("grant_type"))
		assert.Equal(t, "cid", r.PostForm.Get("client_id"))
		assert.Equal(t, "csec", r.PostForm.Get("client_secret"))
		assert.Equal(t, "rtok", r.PostForm.Get("refresh_token"))
		_, _ = w.Write([]byte(`{"access_token":"fresh-token","expires_in":3600}`))
	}))
	defer ts.Close()
	client = zohoHTTPClient{
		httpClient: ts.Client(),
		cfg: &config.Config{
			ZohoAccountsURL:  ts.URL,
			ZohoClientID:     "cid",
			ZohoClientSecret: "csec",
			ZohoRefreshToken: "rtok",
		},
	}

	tok, err := token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", tok)
	assert.Equal(t, 1, requests)
	// Expiry is expires_in minus a 60s safety margin.
	assert.WithinDuration(t, time.Now().Add(3540*time.Second), client.expiry, 5*time.Second)

	// Second call is served from cache without hitting the server.
	tok, err = token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "fresh-token", tok)
	assert.Equal(t, 1, requests)
}

func TestTokenRejectsErrorStatus(t *testing.T) {
	saveClient(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer ts.Close()
	client = zohoHTTPClient{httpClient: ts.Client(), cfg: &config.Config{ZohoAccountsURL: ts.URL}}

	_, err := token(context.Background())
	var se *statusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusBadRequest, se.status)
	assert.Equal(t, `{"error":"invalid_client"}`, se.body)
}

func TestTokenRejectsEmptyAccessToken(t *testing.T) {
	saveClient(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"","expires_in":3600}`))
	}))
	defer ts.Close()
	client = zohoHTTPClient{httpClient: ts.Client(), cfg: &config.Config{ZohoAccountsURL: ts.URL}}

	_, err := token(context.Background())
	assert.ErrorIs(t, err, errEmptyAccessToken)
}

func TestFetchContactsV8SinglePage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/crm/v8/Contacts", r.URL.Path)
		assert.Equal(t, "Zoho-oauthtoken cached-token", r.Header.Get("Authorization"))
		assert.Equal(t, contactFields, r.URL.Query().Get("fields"))
		assert.Equal(t, "1", r.URL.Query().Get("page"))
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"1","First_Name":"Anna","Last_Name":"Muster","Contact_Status":"Active"},
				{"id":"2","First_Name":"Ben","Last_Name":"Beispiel","Contact_Status":"Active"}
			],
			"info": {"more_records": false}
		}`))
	}))
	defer ts.Close()
	setCRMClient(t, ts)

	got, err := fetchContactsV8(context.Background())
	require.NoError(t, err)
	require.Len(t, *got, 2)
	assert.Equal(t, "1", (*got)[0].ID)
	assert.Equal(t, "Ben", (*got)[1].FirstName)
	assert.NotEmpty(t, (*got)[0].Raw)
}

func TestFetchContactsV8Pagination(t *testing.T) {
	var requests int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			assert.Equal(t, "1", r.URL.Query().Get("page"))
			assert.Empty(t, r.URL.Query().Get("page_token"))
			_, _ = w.Write([]byte(`{
				"data": [{"id":"1","First_Name":"Anna","Contact_Status":"Active"}],
				"info": {"more_records": true, "next_page_token": "tok-page-2"}
			}`))
		case 2:
			assert.Equal(t, "tok-page-2", r.URL.Query().Get("page_token"))
			assert.Empty(t, r.URL.Query().Get("page"))
			_, _ = w.Write([]byte(`{
				"data": [{"id":"2","First_Name":"Ben","Contact_Status":"Active"}],
				"info": {"more_records": false}
			}`))
		default:
			t.Error("unexpected extra page request")
		}
	}))
	defer ts.Close()
	setCRMClient(t, ts)

	got, err := fetchContactsV8(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, requests)
	require.Len(t, *got, 2)
	assert.Equal(t, "1", (*got)[0].ID)
	assert.Equal(t, "2", (*got)[1].ID)
}

func TestFetchContactsV8FiltersInactiveContacts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [
				{"id":"1","First_Name":"Anna","Contact_Status":"Active"},
				{"id":"2","First_Name":"Gone","Contact_Status":"Inactive"},
				{"id":"3","First_Name":"NoStatus"}
			],
			"info": {"more_records": false}
		}`))
	}))
	defer ts.Close()
	setCRMClient(t, ts)

	got, err := fetchContactsV8(context.Background())
	require.NoError(t, err)
	require.Len(t, *got, 2)
	assert.Equal(t, "1", (*got)[0].ID)
	assert.Equal(t, "3", (*got)[1].ID)
}

func TestFetchContactsV8NoContentReturnsEmpty(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	setCRMClient(t, ts)

	got, err := fetchContactsV8(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, *got)
}

func TestFetchContactsV8ErrorStatusIncludesBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL_ERROR"}`))
	}))
	defer ts.Close()
	setCRMClient(t, ts)

	_, err := fetchContactsV8(context.Background())
	var se *statusError
	require.ErrorAs(t, err, &se)
	assert.Equal(t, http.StatusInternalServerError, se.status)
	assert.Equal(t, `{"code":"INTERNAL_ERROR"}`, se.body)
}

func TestFetchContactsV8MissingNextPageTokenFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"data": [{"id":"1","Contact_Status":"Active"}],
			"info": {"more_records": true, "next_page_token": ""}
		}`))
	}))
	defer ts.Close()
	setCRMClient(t, ts)

	_, err := fetchContactsV8(context.Background())
	assert.ErrorIs(t, err, errNoNextPageToken)
}

func TestSyncDisabledShortCircuits(t *testing.T) {
	saveClient(t)
	client = zohoHTTPClient{cfg: &config.Config{ZohoSyncEnabled: false}}

	_, err := fetchContacts(context.Background())
	assert.ErrorIs(t, err, ErrSyncDisabled)

	_, err = fetchContactsV8(context.Background())
	assert.ErrorIs(t, err, ErrSyncDisabled)

	_, err = buildZohoRequest(context.Background(), http.MethodGet, "http://example.invalid", nil)
	assert.ErrorIs(t, err, ErrSyncDisabled)
}
