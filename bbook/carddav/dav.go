// Package carddav serves the bbook contact directory as a read-only CardDAV server.

package carddav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
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

var errWebDavReadOnly = webdav.NewHTTPError(http.StatusForbidden, errors.New("bbook address book is read-only"))

var errWebDavInternal = webdav.NewHTTPError(http.StatusInternalServerError, errors.New("internal error"))

// CardDav HTTP handler.
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

func (backend) ListAddressObjects(ctx context.Context, p string, _ *gocarddav.AddressDataRequest) ([]gocarddav.AddressObject, error) {
	if p != addressBookPath {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("listing address objects under %q: no such collection", p))
	}
	contacts, err := search.Search("")
	if err != nil {
		slog.ErrorContext(ctx, "carddav: listing address objects", "err", err)
		return nil, errWebDavInternal
	}
	objs := make([]gocarddav.AddressObject, 0, len(contacts))
	for _, c := range contacts {
		obj, err := toObject(c)
		if err != nil {
			slog.ErrorContext(ctx, "carddav: encoding contact", "id", c.ID, "err", err)
			return nil, errWebDavInternal
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

// Note: we ignore the query and just return everything, while still honoring the limit in the results.
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

func (backend) GetAddressObject(ctx context.Context, p string, _ *gocarddav.AddressDataRequest) (*gocarddav.AddressObject, error) {
	c, ok := search.ByID(idFromPath(p))
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("getting address object %q: no such contact", p))
	}
	obj, err := toObject(c)
	if err != nil {
		slog.ErrorContext(ctx, "carddav: encoding contact", "id", c.ID, "err", err)
		return nil, errWebDavInternal
	}
	return &obj, nil
}

func (backend) CreateAddressBook(context.Context, *gocarddav.AddressBook) error {
	return errWebDavReadOnly
}

func (backend) DeleteAddressBook(context.Context, string) error {
	return errWebDavReadOnly
}

func (backend) PutAddressObject(context.Context, string, vcard.Card, *gocarddav.PutAddressObjectOptions) (*gocarddav.AddressObject, error) {
	return nil, errWebDavReadOnly
}

func (backend) DeleteAddressObject(context.Context, string) error {
	return errWebDavReadOnly
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
	sum := sha256.Sum256(buf.Bytes())

	return gocarddav.AddressObject{
		Path:          addressBookPath + c.ID + ".vcf",
		ContentLength: int64(buf.Len()),
		ETag:          hex.EncodeToString(sum[:]),
		Card:          card,
	}, nil
}
