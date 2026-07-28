package search

import (
	"reflect"
	"testing"

	"git.adfinis.com/int-infrastructure/bbook/bbook/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clears the package-level index and restores the previous one after the test.
func resetIndex(t *testing.T) {
	t.Helper()
	prev := current.Load()
	current.Store(nil)
	t.Cleanup(func() { current.Store(prev) })
}

func testContacts() []database.Contact {
	return []database.Contact{
		{ZohoID: "z1", FirstName: "Alice", LastName: "Smith", Organization: "Acme", Email: "alice@acme.example"},
		{ZohoID: "z2", FirstName: "Bob", LastName: "Jones", Organization: "Globex", Email: "bob@globex.example"},
		{ZohoID: "z3", FirstName: "Carol", LastName: "Miller", Organization: "Acme", Email: "carol@acme.example"},
	}
}

func TestSearchWithoutIndexReturnsNothing(t *testing.T) {
	resetIndex(t)

	res, err := Search("anything")
	require.NoError(t, err)
	assert.Nil(t, res)

	res, err = Search("")
	require.NoError(t, err)
	assert.Nil(t, res)

	_, ok := ByID("z1")
	assert.False(t, ok)
}

func TestSearchEmptyQueryReturnsAllInInsertionOrder(t *testing.T) {
	resetIndex(t)
	contacts := testContacts()
	require.NoError(t, Rebuild(contacts))

	res, err := Search("")
	require.NoError(t, err)
	require.Len(t, res, len(contacts))
	for i, c := range contacts {
		assert.Equal(t, c.ToView(), res[i])
	}
}

func TestSearchMatchesContactFields(t *testing.T) {
	resetIndex(t)
	require.NoError(t, Rebuild(testContacts()))

	tests := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{"first name", "alice", []string{"z1"}},
		{"last name", "jones", []string{"z2"}},
		{"organization", "acme", []string{"z1", "z3"}},
		{"email token", "globex.example", []string{"z2"}},
		{"field scoped email", "email:carol", []string{"z3"}},
		{"scoped field excludes other fields", "lastname:alice", nil},
		{"no match", "nonexistent", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Search(tt.query)
			require.NoError(t, err)
			got := make([]string, 0, len(res))
			for _, c := range res {
				got = append(got, c.ID)
			}
			assert.ElementsMatch(t, tt.wantIDs, got)
		})
	}
}

func TestByIDResolvesIndexedContacts(t *testing.T) {
	resetIndex(t)
	contacts := testContacts()
	require.NoError(t, Rebuild(contacts))

	got, ok := ByID("z2")
	require.True(t, ok)
	assert.Equal(t, contacts[1].ToView(), got)

	_, ok = ByID("missing")
	assert.False(t, ok)
}

func TestRebuildReplacesPreviousIndex(t *testing.T) {
	resetIndex(t)
	require.NoError(t, Rebuild([]database.Contact{{ZohoID: "old", FirstName: "Olivia"}}))
	require.NoError(t, Rebuild([]database.Contact{{ZohoID: "new", FirstName: "Nadia"}}))

	res, err := Search("olivia")
	require.NoError(t, err)
	assert.Empty(t, res)
	_, ok := ByID("old")
	assert.False(t, ok)

	res, err = Search("nadia")
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "new", res[0].ID)
}

func TestRebuildNilGivesEmptyIndex(t *testing.T) {
	resetIndex(t)
	require.NoError(t, Rebuild(nil))

	res, err := Search("")
	require.NoError(t, err)
	assert.Empty(t, res)

	res, err = Search("anything")
	require.NoError(t, err)
	assert.Empty(t, res)
}

func TestComputeIndexedFieldsUsesLowercasedExportedFields(t *testing.T) {
	fields := computeIndexedFields(reflect.TypeOf(database.ContactView{}))
	want := []string{
		"id", "firstname", "lastname", "fullname", "org", "email",
		"phone", "mobile", "street", "city", "postalcode", "address",
	}
	got := make([]string, len(fields))
	for i, f := range fields {
		got[i] = f.DocKey
		assert.Equal(t, i, f.Idx)
	}
	assert.Equal(t, want, got)
	assert.Equal(t, want, FieldNames)
}
