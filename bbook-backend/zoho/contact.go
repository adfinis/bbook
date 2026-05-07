package zoho

import "time"

type Contact struct {
	ID           string
	FirstName    string
	LastName     string
	Email        string
	Phone        string
	Mobile       string
	Organization string
	Street       string
	City         string
	PostalCode   string
	Status       string
	ModifiedTime time.Time
}

func (c Contact) FullName() string {
	switch {
	case c.FirstName == "":
		return c.LastName
	case c.LastName == "":
		return c.FirstName
	default:
		return c.FirstName + " " + c.LastName
	}
}
