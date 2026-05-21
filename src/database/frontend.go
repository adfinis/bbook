package database

import (
	"reflect"
	"strings"
)

// ContactView is the projection of a database record to be handed to the client.
type ContactView struct {
	ID     string `csv:"ID"`
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

// Given a Contact in the database, give its ContactView projection
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
		ID:     c.ZohoID,
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




// csvFields enumerates the ContactView fields tagged with `csv:"..."
type csvField struct {
	Index  int
	Header string
}
// List of the columns for the CSV
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

// ToCSV returns the contact as a single CSV row
func (c ContactView) ToCSV() []string {
	v := reflect.ValueOf(c)
	row := make([]string, len(csvFields))
	for i, f := range csvFields {
		row[i] = v.Field(f.Index).String()
	}
	return row
}



// VCard returns the contact as a vCard string.
func (c ContactView) VCard() string {
	lines := []string{"BEGIN:VCARD", "VERSION:3.0"}
	if c.FullName != "" {
		lines = append(lines, "FN:"+vCardEscape(c.FullName))
	}
	if c.FirstName != "" || c.LastName != "" {
		lines = append(lines, "N:"+vCardEscape(c.LastName)+";"+vCardEscape(c.FirstName)+";;;")
	}
	if c.Org != "" {
		lines = append(lines, "ORG:"+vCardEscape(c.Org))
	}
	if c.Email != "" {
		lines = append(lines, "EMAIL;TYPE=INTERNET:"+c.Email)
	}
	if c.Phone != "" {
		lines = append(lines, "TEL;TYPE=WORK,VOICE:"+c.Phone)
	}
	if c.Mobile != "" {
		lines = append(lines, "TEL;TYPE=CELL:"+c.Mobile)
	}
	if c.Street != "" || c.City != "" || c.PostalCode != "" {
		// ADR fields: po-box;extended;street;locality;region;postal-code;country
		lines = append(lines, "ADR;TYPE=WORK:;;"+
			vCardEscape(c.Street)+";"+
			vCardEscape(c.City)+";;"+
			vCardEscape(c.PostalCode)+";")
	}
	lines = append(lines, "END:VCARD")
	return strings.Join(lines, "\r\n")
}

func vCardEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	return s
}
