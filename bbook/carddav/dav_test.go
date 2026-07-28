package carddav

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
	gocarddav "github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"git.adfinis.com/int-infrastructure/bbook/bbook/server/search"
)

func testContact(id string) database.Contact {
	return database.Contact{
		ZohoID:       id,
		FirstName:    "Ada",
		LastName:     "Lovelace",
		Organization: "Adfinis",
		Email:        id + "@example.com",
		Phone:        "+41 11 111 11 11",
		Mobile:       "+41 79 111 11 11",
		Street:       "Main St 1",
		City:         "Bern",
		PostalCode:   "3000",
	}
}

// seed replaces the shared search index and clears it again after the test.
func seed(t *testing.T, contacts []database.Contact) {
	t.Helper()
	require.NoError(t, search.Rebuild(contacts))
	t.Cleanup(func() {
		require.NoError(t, search.Rebuild(nil))
	})
}

// httpStatus extracts the status code from a webdav HTTPError. The concrete
// type lives in go-webdav's internal package, so we read its Code field via
// reflection.
func httpStatus(t *testing.T, err error) int {
	t.Helper()
	require.Error(t, err)
	v := reflect.ValueOf(err)
	require.Equal(t, reflect.Pointer, v.Kind(), "error %T is not a webdav HTTPError", err)
	f := v.Elem().FieldByName("Code")
	require.True(t, f.IsValid(), "error %T carries no HTTP status code", err)
	return int(f.Int())
}

func TestMutatingOperationsReturnReadOnlyError(t *testing.T) {
	b := backend{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{"CreateAddressBook", func() error {
			return b.CreateAddressBook(ctx, &gocarddav.AddressBook{Path: addressBookPath})
		}},
		{"DeleteAddressBook", func() error {
			return b.DeleteAddressBook(ctx, addressBookPath)
		}},
		{"PutAddressObject", func() error {
			_, err := b.PutAddressObject(ctx, addressBookPath+"x.vcf", vcard.Card{}, nil)
			return err
		}},
		{"DeleteAddressObject", func() error {
			return b.DeleteAddressObject(ctx, addressBookPath+"x.vcf")
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			assert.ErrorIs(t, err, errWebDavReadOnly)
			assert.Equal(t, http.StatusForbidden, httpStatus(t, err))
		})
	}
}

func TestGetAddressBookMatchesExactPathOnly(t *testing.T) {
	b := backend{}
	ctx := context.Background()

	ab, err := b.GetAddressBook(ctx, addressBookPath)
	require.NoError(t, err)
	require.NotNil(t, ab)
	assert.Equal(t, addressBookPath, ab.Path)
	assert.Equal(t, "Adfinis contacts", ab.Name)

	for _, p := range []string{
		strings.TrimSuffix(addressBookPath, "/"),
		homeSetPath,
	} {
		t.Run(p, func(t *testing.T) {
			ab, err := b.GetAddressBook(ctx, p)
			assert.Nil(t, ab)
			assert.Equal(t, http.StatusNotFound, httpStatus(t, err))
		})
	}
}

func TestListAddressObjectsReturnsSeededContacts(t *testing.T) {
	contacts := []database.Contact{testContact("1001"), testContact("1002")}
	seed(t, contacts)
	b := backend{}

	_, err := b.ListAddressObjects(context.Background(), homeSetPath, nil)
	assert.Equal(t, http.StatusNotFound, httpStatus(t, err))

	objs, err := b.ListAddressObjects(context.Background(), addressBookPath, nil)
	require.NoError(t, err)
	require.Len(t, objs, len(contacts))
	for i, c := range contacts {
		obj := objs[i]
		vcf := c.ToView().VCardString()
		sum := sha256.Sum256([]byte(vcf))
		assert.Equal(t, addressBookPath+c.ZohoID+".vcf", obj.Path)
		assert.Equal(t, hex.EncodeToString(sum[:]), obj.ETag, "ETag should be the sha256 of the encoded vCard")
		assert.Equal(t, int64(len(vcf)), obj.ContentLength)
		assert.Equal(t, "bbook-"+c.ZohoID, obj.Card.Value(vcard.FieldUID))
	}
}

func TestListAddressObjectsEmptyIndex(t *testing.T) {
	seed(t, nil)

	objs, err := backend{}.ListAddressObjects(context.Background(), addressBookPath, nil)
	require.NoError(t, err)
	assert.Empty(t, objs)
}

func TestQueryAddressObjectsHonorsLimit(t *testing.T) {
	seed(t, []database.Contact{testContact("1"), testContact("2"), testContact("3")})
	b := backend{}

	tests := []struct {
		name  string
		query *gocarddav.AddressBookQuery
		want  int
	}{
		{"limit below count", &gocarddav.AddressBookQuery{Limit: 2}, 2},
		{"limit zero is unlimited", &gocarddav.AddressBookQuery{Limit: 0}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			objs, err := b.QueryAddressObjects(context.Background(), addressBookPath, tc.query)
			require.NoError(t, err)
			assert.Len(t, objs, tc.want)
		})
	}
}

func TestGetAddressObjectResolvesByID(t *testing.T) {
	seed(t, []database.Contact{testContact("4711")})
	b := backend{}

	obj, err := b.GetAddressObject(context.Background(), addressBookPath+"4711.vcf", nil)
	require.NoError(t, err)
	assert.Equal(t, addressBookPath+"4711.vcf", obj.Path)
	assert.Equal(t, "bbook-4711", obj.Card.Value(vcard.FieldUID))

	obj, err = b.GetAddressObject(context.Background(), addressBookPath+"unknown.vcf", nil)
	assert.Nil(t, obj)
	assert.Equal(t, http.StatusNotFound, httpStatus(t, err))
}

func TestToObjectETagTracksContent(t *testing.T) {
	c := testContact("9").ToView()

	a, err := toObject(c)
	require.NoError(t, err)
	b, err := toObject(c)
	require.NoError(t, err)
	assert.Equal(t, a.ETag, b.ETag)

	c.Email = "changed@example.com"
	changed, err := toObject(c)
	require.NoError(t, err)
	assert.NotEqual(t, a.ETag, changed.ETag)
}

func TestHandlerRejectsPutOverHTTP(t *testing.T) {
	seed(t, []database.Contact{testContact("7")})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, addressBookPath+"7.vcf", strings.NewReader("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:bbook-7\r\nEND:VCARD\r\n"))
	req.Header.Set("Content-Type", vcard.MIMEType)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandlerAdvertisesAddressBookSupport(t *testing.T) {
	seed(t, []database.Contact{testContact("7")})

	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, addressBookPath, nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Contains(t, rec.Header().Get("DAV"), "addressbook")
}

func TestHandlerServesPropfindOnAddressBook(t *testing.T) {
	seed(t, []database.Contact{testContact("7")})

	req := httptest.NewRequestWithContext(t.Context(), "PROPFIND", addressBookPath, nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMultiStatus, rec.Code)
}
