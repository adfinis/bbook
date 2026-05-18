package search

import (
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	"git.adfinis.com/albertc/bbook/bbook-backend/database"
	"github.com/blevesearch/bleve/v2"
)

// field describes one database.ContactView field that bleve indexes.
type field struct {
	DocKey string
	Idx    int
}

// list of all Bleve searchable fields. Auto generated from database.ContactView
var indexedFields = computeIndexedFields(reflect.TypeOf(database.ContactView{}))

// list of the possible field names
var FieldNames = func() []string {
	names := make([]string, len(indexedFields))
	for i, f := range indexedFields {
		names[i] = f.DocKey
	}
	return names
}()

func computeIndexedFields(t reflect.Type) []field {
	var out []field
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		out = append(out, field{
			DocKey: strings.ToLower(f.Name),
			Idx:    i,
		})
	}
	return out
}

// Holds Bleve's state
type state struct {
	idx     bleve.Index                     // the bleve full-text index
	byID    map[string]database.ContactView // resolves a hit's ID back to its ContactView
	ordered []database.ContactView          // the unfiltered list returned when q is empty
}

var current atomic.Pointer[state]

// Rebuilds the search index given a list of contacts
func Rebuild(contacts []database.Contact) error {
	idx, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		return fmt.Errorf("rebuilding bleve index: new index: %w", err)
	}

	byID := make(map[string]database.ContactView, len(contacts))
	ordered := make([]database.ContactView, 0, len(contacts))
	batch := idx.NewBatch()
	for _, c := range contacts {
		fc := c.ToView()
		if err := batch.Index(c.ZohoID, contactDoc(fc)); err != nil {
			return fmt.Errorf("rebuilding bleve index: batch contact %s: %w", c.ZohoID, err)
		}
		byID[c.ZohoID] = fc
		ordered = append(ordered, fc)
	}
	if err := idx.Batch(batch); err != nil {
		return fmt.Errorf("rebuilding bleve index: commit batch: %w", err)
	}

	current.Store(&state{idx: idx, byID: byID, ordered: ordered})
	return nil
}

// Search returns contacts matching the Lucene-style query q.
func Search(q string) ([]database.ContactView, error) {
	s := current.Load()
	if s == nil {
		return nil, nil
	}
	// Empty query returns everything
	if q == "" {
		return s.ordered, nil
	}

	req := bleve.NewSearchRequest(bleve.NewQueryStringQuery(q))
	req.Size = len(s.ordered)

	res, err := s.idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("searching bleve: %w", err)
	}

	results := make([]database.ContactView, 0, len(res.Hits))
	for _, hit := range res.Hits {
		if c, ok := s.byID[hit.ID]; ok {
			results = append(results, c)
		}
	}
	return results, nil
}

func contactDoc(c database.ContactView) map[string]any {
	rv := reflect.ValueOf(c)
	doc := make(map[string]any, len(indexedFields))
	for _, f := range indexedFields {
		doc[f.DocKey] = rv.Field(f.Idx).Interface()
	}
	return doc
}
