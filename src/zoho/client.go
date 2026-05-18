package zoho

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"git.adfinis.com/albertc/bbook/bbook-backend/config"
	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"git.adfinis.com/albertc/bbook/bbook-backend/server/search"
)

var client zohoHTTPClient = zohoHTTPClient{}

type zohoHTTPClient struct {
	httpClient  *http.Client
	accessToken string
	expiry      time.Time
}

func Init() error {
	client.httpClient = &http.Client{Timeout: 30 * time.Second}
	return nil
}

// Fetches contacts from ZohoCRM and upserts them into the database, while deleting unavailable contacts.
func RunZohoSync(ctx context.Context) error {
	runStart := time.Now()
	contacts, err := fetchContacts(ctx)
	if err != nil {
		return err
	}

	tx, err := database.Client.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after a successful Commit

	q := database.Client.Queries.WithTx(tx)
	for _, c := range contacts {
		c.SyncedAt = time.Now()
		if err := q.UpsertContact(ctx, database.UpsertContactParams(c)); err != nil {
			return err
		}
	}

	// Delete contacts that weren't synced by this run (i.e were deleted since the last sync)
	deleted, err := q.DeleteContactsSyncedBefore(ctx, runStart)
	if err != nil {
		return err
	}
	log.Printf("zoho sync: upserted %d, deleted %d stale", len(contacts), deleted)
	if err := tx.Commit(); err != nil {
		return err
	}

	// Refresh the search index with the freshly-synced contacts.
	if err := search.Rebuild(contacts); err != nil {
		log.Printf("zoho sync: rebuild search index: %v", err)
	}
	return nil
}


// Fetches all contacts from Zoho
func fetchContacts(ctx context.Context) ([]database.Contact, error) {
	var all []database.Contact

	rawContacts, err := fetchContactsV8(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching zoho contacts: %w", err)
	}
	for _, rc := range *rawContacts {
		all = append(all, rc.toContact())
	}

	return all, nil
}

// https://www.zoho.com/crm/developer/docs/api/v8/get-records.html
func fetchContactsV8(ctx context.Context) (*[]rawZohoContact, error) {
	var prevPage rawContactsPage
	res := &[]rawZohoContact{}
	pageNum := 1
	for ; ; pageNum++ {
		u := fmt.Sprintf("%s/crm/v8/Contacts?fields=%s", config.AppConfig.ZohoBaseURL, contactFields)
		if pageNum == 1 {
			u += fmt.Sprintf("&page=%d", pageNum)
		} else {
			// We use page_token for retrieving any subsequent page after the first one
			u += "&page_token=" + url.QueryEscape(prevPage.Info.NextPageToken)
		}
		// fmt.Println(u)

		req, err := buildZohoRequest(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching zoho contacts page %d: %w", pageNum, err)
		}

		resp, err := client.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching zoho contacts page %d: %w", pageNum, err)
		}
		defer resp.Body.Close()

		// No more content
		if resp.StatusCode == http.StatusNoContent {
			return res, nil
		}
		// Any other issue
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("fetching zoho contacts page %d: status %d: %s", pageNum, resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var page rawContactsPage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return nil, fmt.Errorf("fetching zoho contacts page %d: decode page: %w", pageNum, err)
		}

		// log.Printf("fetched %d contacts for page %d", len(page.Data), pageNum)
		for _, raw := range page.Data {
			var rc rawZohoContact
			if err := json.Unmarshal(raw, &rc); err != nil {
				return nil, fmt.Errorf("fetching zoho contacts page %d: decode contact: %w", pageNum, err)
			}
			*res = append(*res, rc)
		}
		if !page.Info.MoreRecords {
			break
		}
		prevPage = page
	}
	log.Printf("fetched %d contacts in %d pages", len(*res), pageNum)
	return res, nil
}

func buildZohoRequest(ctx context.Context, method string, url string, body io.Reader) (*http.Request, error) {
	token, err := token(ctx)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	return req, nil
}

// Generated comma-separated list of JSON field names for rawZohoContact. When fetching contacts from Zoho, we only request these fields.
var contactFields = buildJSONFields(reflect.TypeFor[rawZohoContact]())

func buildJSONFields(t reflect.Type) string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}
		if tag == "" || tag == "-" || tag == "id" {
			continue // skip untagged, ignored
		}
		names = append(names, tag)
	}
	return strings.Join(names, ",")
}

type rawContactsPage struct {
	Data []json.RawMessage `json:"data"`
	Info struct {
		MoreRecords   bool   `json:"more_records"`
		NextPageToken string `json:"next_page_token"`
	} `json:"info"`
}

type rawZohoContact struct {
	ID          string `json:"id"`
	FirstName   string `json:"First_Name"`
	LastName    string `json:"Last_Name"`
	Email       string `json:"Email"`
	Phone       string `json:"Phone"`
	Mobile      string `json:"Mobile"`
	AccountName *struct {
		Name string `json:"name"`
		Id   string `json:"id"`
	} `json:"Account_Name"`
	MailingStreet string          `json:"Mailing_Street"`
	MailingCity   string          `json:"Mailing_City"`
	MailingZip    string          `json:"Mailing_Zip"`
	Status        string          `json:"Contact_Status"`
	ModifiedTime  time.Time       `json:"Modified_Time"`
	Raw           json.RawMessage `json:"-"`
}

func (r *rawZohoContact) UnmarshalJSON(data []byte) error {
	type alias rawZohoContact
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = rawZohoContact(a)
	r.Raw = data
	return nil
}

func (r rawZohoContact) toContact() database.Contact {
	org := ""
	if r.AccountName != nil {
		org = r.AccountName.Name
	}
	return database.Contact{
		ZohoID:       r.ID,
		FirstName:    r.FirstName,
		LastName:     r.LastName,
		Organization: org,
		Email:        r.Email,
		Phone:        r.Phone,
		Mobile:       r.Mobile,
		Street:       r.MailingStreet,
		City:         r.MailingCity,
		PostalCode:   r.MailingZip,
		Status:       r.Status,
		ModifiedTime: r.ModifiedTime,
		Raw:          r.Raw,
	}
}


// Returns (and refreshes if necessary) the access token
func token(ctx context.Context) (string, error) {
	if client.accessToken != "" && time.Now().Before(client.expiry) {
		return client.accessToken, nil
	}

	// Token needs to be refreshed

	form := url.Values{}
	form.Set("client_id", config.AppConfig.ZohoClientID)
	form.Set("client_secret", config.AppConfig.ZohoClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", config.AppConfig.ZohoRefreshToken)

	endpoint := config.AppConfig.ZohoAccountsURL + "/oauth/v2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("refreshing zoho access token: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("refreshing zoho access token: %w", err)
	}
	defer resp.Body.Close()

	// Refresh successful
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("refreshing zoho access token: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("refreshing zoho access token: decode response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("refreshing zoho access token: empty access_token in response")
	}

	client.accessToken = tr.AccessToken
	client.expiry = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	return client.accessToken, nil
}
