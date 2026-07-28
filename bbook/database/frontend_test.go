package database

import (
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToViewComposesAddress(t *testing.T) {
	tests := []struct {
		name    string
		street  string
		zip     string
		city    string
		address string
	}{
		{"street zip city", "Main St 1", "8000", "Zurich", "Main St 1, 8000 Zurich"},
		{"street only", "Main St 1", "", "", "Main St 1"},
		{"street and city", "Main St 1", "", "Zurich", "Main St 1, Zurich"},
		{"zip and city", "", "8000", "Zurich", "8000 Zurich"},
		{"all empty", "", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Contact{Street: tc.street, PostalCode: tc.zip, City: tc.city}
			assert.Equal(t, tc.address, c.ToView().Address)
		})
	}
}

func TestToViewComposesFullName(t *testing.T) {
	tests := []struct {
		name     string
		first    string
		last     string
		fullName string
	}{
		{"both names", "Ada", "Lovelace", "Ada Lovelace"},
		{"first only", "Ada", "", "Ada"},
		{"last only", "", "Lovelace", "Lovelace"},
		{"neither", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := Contact{FirstName: tc.first, LastName: tc.last}
			assert.Equal(t, tc.fullName, c.ToView().FullName)
		})
	}
}

func fullContact() Contact {
	return Contact{
		ZohoID:       "z123",
		FirstName:    "Ada",
		LastName:     "Lovelace",
		Organization: "Adfinis",
		Email:        "ada@example.com",
		Phone:        "+41 44 000 00 00",
		Mobile:       "+41 79 000 00 00",
		Street:       "Main St 1",
		City:         "Zurich",
		PostalCode:   "8000",
	}
}

func TestToViewMapsFields(t *testing.T) {
	v := fullContact().ToView()

	assert.Equal(t, "z123", v.ID)
	assert.Equal(t, "Ada", v.FirstName)
	assert.Equal(t, "Lovelace", v.LastName)
	assert.Equal(t, "Adfinis", v.Org)
	assert.Equal(t, "ada@example.com", v.Email)
	assert.Equal(t, "+41 44 000 00 00", v.Phone)
	assert.Equal(t, "+41 79 000 00 00", v.Mobile)
	assert.Equal(t, "Main St 1", v.Street)
	assert.Equal(t, "Zurich", v.City)
	assert.Equal(t, "8000", v.PostalCode)
}

func TestVCardPopulatedFields(t *testing.T) {
	card := fullContact().ToView().VCard()

	assert.Equal(t, "3.0", card.Value(vcard.FieldVersion))
	assert.Equal(t, "bbook-z123", card.Value(vcard.FieldUID))
	assert.Equal(t, "Ada Lovelace", card.Value(vcard.FieldFormattedName))
	assert.Equal(t, "Adfinis", card.Value(vcard.FieldOrganization))

	name := card.Name()
	require.NotNil(t, name)
	assert.Equal(t, "Lovelace", name.FamilyName)
	assert.Equal(t, "Ada", name.GivenName)

	email := card.Get(vcard.FieldEmail)
	require.NotNil(t, email)
	assert.Equal(t, "ada@example.com", email.Value)
	assert.Equal(t, []string{"internet"}, email.Params[vcard.ParamType])

	phones := card[vcard.FieldTelephone]
	require.Len(t, phones, 2)
	assert.Equal(t, "+41 44 000 00 00", phones[0].Value)
	assert.Equal(t, []string{vcard.TypeWork, vcard.TypeVoice}, phones[0].Params[vcard.ParamType])
	assert.Equal(t, "+41 79 000 00 00", phones[1].Value)
	assert.Equal(t, []string{vcard.TypeCell}, phones[1].Params[vcard.ParamType])

	addr := card.Address()
	require.NotNil(t, addr)
	assert.Equal(t, "Main St 1", addr.StreetAddress)
	assert.Equal(t, "Zurich", addr.Locality)
	assert.Equal(t, "8000", addr.PostalCode)
}

func TestVCardOmitsEmptyFields(t *testing.T) {
	card := ContactView{ID: "z123"}.VCard()

	assert.Equal(t, "3.0", card.Value(vcard.FieldVersion))
	assert.Equal(t, "bbook-z123", card.Value(vcard.FieldUID))

	for _, field := range []string{
		vcard.FieldFormattedName,
		vcard.FieldName,
		vcard.FieldOrganization,
		vcard.FieldEmail,
		vcard.FieldTelephone,
		vcard.FieldAddress,
	} {
		assert.Nil(t, card.Get(field), "field %q should be absent", field)
	}
}

func TestVCardStringEncodesWithCRLF(t *testing.T) {
	s := fullContact().ToView().VCardString()
	assert.True(t, strings.HasPrefix(s, "BEGIN:VCARD\r\n"))
	assert.True(t, strings.HasSuffix(s, "END:VCARD\r\n"))
	assert.Contains(t, s, "UID:bbook-z123\r\n")
	assert.Contains(t, s, "VERSION:3.0\r\n")
}
