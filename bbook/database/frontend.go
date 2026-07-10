package database

import (
	"reflect"
	"strings"

	"github.com/emersion/go-vcard"
)

// ContactView is the projection of a database record to be handed to the client.
type ContactView struct {
	ID         string `csv:"ID"`
	FirstName  string `csv:"First Name"`
	LastName   string `csv:"Last Name"`
	FullName   string
	Org        string `csv:"Organization"`
	Email      string `csv:"Email"`
	Phone      string `csv:"Phone"`
	Mobile     string `csv:"Mobile"`
	Street     string `csv:"Street"`
	City       string `csv:"City"`
	PostalCode string `csv:"Postal Code"`
	Address    string
}

// Given a Contact in the database, give its ContactView projection.
func (c Contact) ToView() ContactView {
	address := c.Street
	if address != "" && (c.City != "" || c.PostalCode != "") {
		address += ","
	}
	if c.PostalCode != "" {
		address += " " + c.PostalCode
	}
	if c.City != "" {
		address += " " + c.City
	}

	return ContactView{
		ID:         c.ZohoID,
		FirstName:  c.FirstName,
		LastName:   c.LastName,
		FullName:   strings.TrimSpace(c.FirstName + " " + c.LastName),
		Org:        c.Organization,
		Email:      c.Email,
		Phone:      c.Phone,
		Mobile:     c.Mobile,
		Street:     c.Street,
		City:       c.City,
		PostalCode: c.PostalCode,
		Address:    address,
	}
}

// csvFields enumerates the ContactView fields tagged with `csv:"..."`.
type csvField struct {
	Index  int
	Header string
}

// List of the columns for the CSV.
var csvFields = func() []csvField {
	var out []csvField
	t := reflect.TypeOf(ContactView{})
	for i := 0; i < t.NumField(); i++ {
		if tag := t.Field(i).Tag.Get("csv"); tag != "" && tag != "-" {
			out = append(out, csvField{Index: i, Header: tag})
		}
	}
	return out
}()

// ContactCSVHeader returns the column titles for CSV export.
func ContactCSVHeader() []string {
	headers := make([]string, len(csvFields))
	for i, f := range csvFields {
		headers[i] = f.Header
	}
	return headers
}

// ToCSV returns the contact as a single CSV row.
func (c ContactView) ToCSV() []string {
	v := reflect.ValueOf(c)
	row := make([]string, len(csvFields))
	for i, f := range csvFields {
		row[i] = v.Field(f.Index).String()
	}
	return row
}

// VCard returns the contact as a vCard 3.0 card.
func (c ContactView) VCard() vcard.Card {
	card := vcard.Card{}
	card.SetValue(vcard.FieldVersion, "3.0")
	card.SetValue(vcard.FieldUID, "bbook-"+c.ID)

	if c.FullName != "" {
		card.SetValue(vcard.FieldFormattedName, c.FullName)
	}
	if c.FirstName != "" || c.LastName != "" {
		card.SetName(&vcard.Name{FamilyName: c.LastName, GivenName: c.FirstName})
	}
	if c.Org != "" {
		card.SetValue(vcard.FieldOrganization, c.Org)
	}
	if c.Email != "" {
		card.Add(vcard.FieldEmail, &vcard.Field{
			Value:  c.Email,
			Params: vcard.Params{vcard.ParamType: {"internet"}},
		})
	}
	if c.Phone != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  c.Phone,
			Params: vcard.Params{vcard.ParamType: {vcard.TypeWork, vcard.TypeVoice}},
		})
	}
	if c.Mobile != "" {
		card.Add(vcard.FieldTelephone, &vcard.Field{
			Value:  c.Mobile,
			Params: vcard.Params{vcard.ParamType: {vcard.TypeCell}},
		})
	}
	if c.Street != "" || c.City != "" || c.PostalCode != "" {
		card.SetAddress(&vcard.Address{
			StreetAddress: c.Street,
			Locality:      c.City,
			PostalCode:    c.PostalCode,
		})
	}
	return card
}

// VCardString returns the contact as a vCard 3.0 string.
func (c ContactView) VCardString() string {
	var buf strings.Builder
	if err := vcard.NewEncoder(&buf).Encode(c.VCard()); err != nil {
		return ""
	}
	return buf.String()
}
