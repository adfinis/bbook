package database

import "strings"

// ContactView is the projection of a database record to be displayed to the user. Strips away any non-relevant fields.
type ContactView struct {
	FirstName string
	LastName  string
	FullName  string
	Org       string
	Email     string
	Phone     string
	Mobile    string
	Address   string
}

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
		FirstName: c.FirstName,
		LastName:  c.LastName,
		FullName:  strings.TrimSpace(c.FirstName + " " + c.LastName),
		Org:       c.Organization,
		Email:     c.Email,
		Phone:     c.Phone,
		Mobile:    c.Mobile,
		Address:   address,
	}
}
