package database

import (
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testStreet = "Main St 1"
	testCity   = "Zurich"
	testFirst  = "Ada"
	testLast   = "Lovelace"
)

func TestToViewComposesAddress(t *testing.T) {
	tests := []struct {
		name    string
		street  string
		zip     string
		city    string
		address string
	}{
		{"street zip city", testStreet, "8000", testCity, testStreet + ", 8000 " + testCity},
		{"street only", testStreet, "", "", testStreet},
		{"street and city", testStreet, "", testCity, testStreet + ", " + testCity},
		{"zip and city", "", "8000", testCity, "8000 " + testCity},
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
		{"both names", testFirst, testLast, testFirst + " " + testLast},
		{"first only", testFirst, "", testFirst},
		{"last only", "", testLast, testLast},
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
		FirstName:    testFirst,
		LastName:     testLast,
		Organization: "Adfinis",
		Email:        "ada@example.com",
		Phone:        "+41 44 000 00 00",
		Mobile:       "+41 79 000 00 00",
		Street:       testStreet,
		City:         testCity,
		PostalCode:   "8000",
	}
}

func TestToViewMapsFields(t *testing.T) {
	v := fullContact().ToView()

	assert.Equal(t, "z123", v.ID)
	assert.Equal(t, testFirst, v.FirstName)
	assert.Equal(t, testLast, v.LastName)
	assert.Equal(t, "Adfinis", v.Org)
	assert.Equal(t, "ada@example.com", v.Email)
	assert.Equal(t, "+41 44 000 00 00", v.Phone)
	assert.Equal(t, "+41 79 000 00 00", v.Mobile)
	assert.Equal(t, testStreet, v.Street)
	assert.Equal(t, testCity, v.City)
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
	assert.Equal(t, testLast, name.FamilyName)
	assert.Equal(t, testFirst, name.GivenName)

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
	assert.Equal(t, testStreet, addr.StreetAddress)
	assert.Equal(t, testCity, addr.Locality)
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
