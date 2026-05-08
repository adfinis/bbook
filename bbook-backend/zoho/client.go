package zoho

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/config"
	"git.sos.ethz.ch/vsos/bbook.vsos.ethz.ch/bbook-backend/database"
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

	var tr tokenResponse
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

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// Fetches all contacts from Zoho
func FetchContacts(ctx context.Context) ([]database.Contact, error) {
	var all []database.Contact

	for pageNum := 1; ; pageNum++ {
		page, err := fetchPage(ctx, pageNum)
		if err != nil {
			return nil, err
		}
		if page == nil {
			break
		}
		for _, raw := range page.Data {
			var rc rawZohoContact
			if err := json.Unmarshal(raw, &rc); err != nil {
				return nil, fmt.Errorf("fetching zoho contacts page %d: decode contact: %w", pageNum, err)
			}
			all = append(all, rc.toContact(raw))
		}
		if !page.Info.MoreRecords {
			break
		}
	}

	return all, nil
}

func fetchPage(ctx context.Context, pageNum int) (*contactsPage, error) {
	token, err := token(ctx)
	if err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/crm/v2/Contacts?page=%d", config.AppConfig.ZohoBaseURL, pageNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching zoho contacts page %d: build request: %w", pageNum, err)
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching zoho contacts page %d: %w", pageNum, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetching zoho contacts page %d: status %d: %s", pageNum, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var page contactsPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("fetching zoho contacts page %d: decode page: %w", pageNum, err)
	}
	return &page, nil
}

type contactsPage struct {
	Data []json.RawMessage `json:"data"`
	Info struct {
		MoreRecords bool `json:"more_records"`
	} `json:"info"`
}

type rawZohoContact struct {
	ID            string    `json:"id"`
	FirstName     string    `json:"First_Name"`
	LastName      string    `json:"Last_Name"`
	Email         string    `json:"Email"`
	Phone         string    `json:"Phone"`
	Mobile        string    `json:"Mobile"`
	AccountName   *account  `json:"Account_Name"`
	MailingStreet string    `json:"Mailing_Street"`
	MailingCity   string    `json:"Mailing_City"`
	MailingZip    string    `json:"Mailing_Zip"`
	Status        string    `json:"Contact_Status"`
	ModifiedTime  time.Time `json:"Modified_Time"`
}

type account struct {
	Name string `json:"name"`
}

func (r rawZohoContact) toContact(raw json.RawMessage) database.Contact {
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
		Raw:          raw,
	}
}
