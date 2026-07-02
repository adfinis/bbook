// Package carddav serves the bbook contact directory as a read-only CardDAV server.

package carddav

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	gocarddav "github.com/emersion/go-webdav/carddav"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"

	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server/search"
)

const Prefix = "/api/dav"

const (
	principalPath   = Prefix + "/principal/"
	homeSetPath     = Prefix + "/principal/addressbooks/"
	addressBookPath = Prefix + "/principal/addressbooks/contacts/"
)

var webDavReadOnlyError = webdav.NewHTTPError(http.StatusForbidden, errors.New("bbook address book is read-only"))

// CardDav HTTP handler
func Handler() http.Handler {
	return &gocarddav.Handler{Backend: backend{}, Prefix: Prefix}
}

type backend struct{}

func (backend) CurrentUserPrincipal(context.Context) (string, error) {
	return principalPath, nil
}

func (backend) AddressBookHomeSetPath(context.Context) (string, error) {
	return homeSetPath, nil
}

func addressBook() gocarddav.AddressBook {
	return gocarddav.AddressBook{
		Path:        addressBookPath,
		Name:        "Adfinis contacts",
		Description: "bbook customer contacts",
		SupportedAddressData: []gocarddav.AddressDataType{
			{ContentType: vcard.MIMEType, Version: "3.0"},
		},
	}
}

func (backend) ListAddressBooks(context.Context) ([]gocarddav.AddressBook, error) {
	return []gocarddav.AddressBook{addressBook()}, nil
}

func (backend) GetAddressBook(_ context.Context, p string) (*gocarddav.AddressBook, error) {
	if p != addressBookPath {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("getting address book %q: no such collection", p))
	}
	ab := addressBook()
	return &ab, nil
}

func (backend) ListAddressObjects(_ context.Context, p string, _ *gocarddav.AddressDataRequest) ([]gocarddav.AddressObject, error) {
	if p != addressBookPath {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("listing address objects under %q: no such collection", p))
	}
	contacts, err := search.Search("")
	if err != nil {
		return nil, fmt.Errorf("listing address objects: %w", err)
	}
	objs := make([]gocarddav.AddressObject, 0, len(contacts))
	for _, c := range contacts {
		obj, err := toObject(c)
		if err != nil {
			return nil, fmt.Errorf("listing address objects: contact %s: %w", c.ID, err)
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

// Note: we ignore the query and just return everything, while still honoring the limit in the results
func (b backend) QueryAddressObjects(ctx context.Context, p string, query *gocarddav.AddressBookQuery) ([]gocarddav.AddressObject, error) {
	objs, err := b.ListAddressObjects(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	if query != nil && query.Limit > 0 && query.Limit < len(objs) {
		objs = objs[:query.Limit]
	}
	return objs, nil
}

func (backend) GetAddressObject(_ context.Context, p string, _ *gocarddav.AddressDataRequest) (*gocarddav.AddressObject, error) {
	c, ok := search.ByID(idFromPath(p))
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("getting address object %q: no such contact", p))
	}
	obj, err := toObject(c)
	if err != nil {
		return nil, fmt.Errorf("getting address object %q: %w", p, err)
	}
	return &obj, nil
}

func (backend) CreateAddressBook(context.Context, *gocarddav.AddressBook) error {
	return webDavReadOnlyError
}

func (backend) DeleteAddressBook(context.Context, string) error {
	return webDavReadOnlyError
}

func (backend) PutAddressObject(context.Context, string, vcard.Card, *gocarddav.PutAddressObjectOptions) (*gocarddav.AddressObject, error) {
	return nil, webDavReadOnlyError
}

func (backend) DeleteAddressObject(context.Context, string) error {
	return webDavReadOnlyError
}

func idFromPath(p string) string {
	return strings.TrimSuffix(path.Base(p), ".vcf")
}

func toObject(c database.ContactView) (gocarddav.AddressObject, error) {
	card := c.VCard()

	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return gocarddav.AddressObject{}, fmt.Errorf("encoding vcard: %w", err)
	}
	sum := sha1.Sum(buf.Bytes())

	return gocarddav.AddressObject{
		Path:          addressBookPath + c.ID + ".vcf",
		ContentLength: int64(buf.Len()),
		ETag:          hex.EncodeToString(sum[:]),
		Card:          card,
	}, nil
}
