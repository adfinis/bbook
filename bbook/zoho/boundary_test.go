package zoho

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests the conversion equivalence of a Zoho contact between representations
// API JSON body -> rawZohoContact -> database.Contact -> ContactView -> vCard/CSV.
func TestBoundaryRawZohoJSONToContactView(t *testing.T) {
	raw := []byte(`{
		"id": "1234567890",
		"First_Name": "  Anna\tMarie ",
		"Last_Name": "Muster\u0000mann",
		"Email": "anna@example.com",
		"Phone": "+41 44 123 45 67",
		"Mobile": "+41 79 987 65 43",
		"Account_Name": {"name": "Adfinis AG", "id": "999"},
		"Mailing_Street": "Bahnhofstrasse 1",
		"Mailing_City": "Zürich",
		"Mailing_Zip": "8001",
		"Contact_Status": "Active",
		"Modified_Time": "2026-05-01T12:00:00+02:00"
	}`)

	var rc rawZohoContact
	require.NoError(t, json.Unmarshal(raw, &rc))

	contact := rc.toContact()
	assert.Equal(t, "1234567890", contact.ZohoID)
	assert.Equal(t, "Anna Marie", contact.FirstName, "control chars and padding sanitized")
	assert.Equal(t, "Muster mann", contact.LastName, "NUL becomes a space in the field value")
	assert.Equal(t, "+41441234567", contact.Phone, "phone spaces removed")
	assert.Equal(t, "+41799876543", contact.Mobile)
	assert.Equal(t, "Adfinis AG", contact.Organization)
	assert.Equal(t, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), contact.ModifiedTime.UTC())
	// Raw is stored NUL-free and stays valid JSON for the jsonb column.
	assert.False(t, bytes.Contains(contact.Raw, []byte(`\u0000`)))
	assert.Contains(t, string(contact.Raw), `"Mustermann"`)
	assert.True(t, json.Valid(contact.Raw))

	view := contact.ToView()
	assert.Equal(t, "1234567890", view.ID, "view ID comes from the Zoho ID")
	assert.Equal(t, "Anna Marie Muster mann", view.FullName)
	assert.Equal(t, "Bahnhofstrasse 1, 8001 Zürich", view.Address, "street, zip city")
	assert.Equal(t, "Adfinis AG", view.Org)

	vcf := view.VCardString()
	assert.Contains(t, vcf, "UID:bbook-1234567890")
	assert.Contains(t, vcf, "FN:Anna Marie Muster mann")
	assert.Contains(t, vcf, "Muster mann;Anna Marie")
	assert.Contains(t, vcf, "ORG:Adfinis AG")
	assert.Contains(t, vcf, "+41441234567")
	assert.Contains(t, vcf, "+41799876543")
	assert.Contains(t, vcf, "anna@example.com")
	assert.Contains(t, vcf, "Bahnhofstrasse 1")

	csv := view.ToCSV()
	assert.Equal(t, []string{
		"1234567890", "Anna Marie", "Muster mann", "Adfinis AG",
		"anna@example.com", "+41441234567", "+41799876543",
		"Bahnhofstrasse 1", "Zürich", "8001",
	}, csv)
}

func TestBoundaryNilAccountNameAndSparseFields(t *testing.T) {
	raw := []byte(`{
		"id": "555",
		"First_Name": "Ben",
		"Last_Name": "",
		"Account_Name": null,
		"Mailing_Street": "",
		"Mailing_City": "Bern",
		"Contact_Status": "Active",
		"Modified_Time": "2026-01-15T08:30:00Z"
	}`)

	var rc rawZohoContact
	require.NoError(t, json.Unmarshal(raw, &rc))

	view := rc.toContact().ToView()
	assert.Empty(t, view.Org, "null Account_Name maps to empty organization")
	assert.Equal(t, "Ben", view.FullName, "full name trimmed when last name is empty")
	assert.Equal(t, "Bern", view.Address, "no comma without a street")
}
