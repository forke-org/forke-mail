package contacts

import (
	"encoding/csv"
	"errors"
	"io"
	"strings"
)

const maxImportRows = 50000

var errTooManyRows = errors.New("too_many_rows")

type Row struct {
	Email     string
	FirstName string
	LastName  string
}

type Parsed struct {
	Rows       []Row
	Invalid    int
	Duplicates int
}

type columns struct {
	email, first, last int
}

func ParseCSV(src io.Reader) (Parsed, error) {
	r := csv.NewReader(src)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	var p Parsed
	cols := columns{email: 0, first: 1, last: 2}
	seen := make(map[string]bool)
	for line := 0; ; line++ {
		record, err := r.Read()
		if err == io.EOF {
			return p, nil
		}
		if err != nil {
			return p, err
		}
		if line == 0 {
			if header, ok := headerColumns(record); ok {
				cols = header
				continue
			}
		}
		if len(p.Rows)+p.Invalid+p.Duplicates >= maxImportRows {
			return p, errTooManyRows
		}
		email := NormalizeEmail(field(record, cols.email))
		switch {
		case !ValidEmail(email):
			p.Invalid++
		case seen[email]:
			p.Duplicates++
		default:
			seen[email] = true
			p.Rows = append(p.Rows, Row{
				Email:     email,
				FirstName: normalizeName(field(record, cols.first)),
				LastName:  normalizeName(field(record, cols.last)),
			})
		}
	}
}

func headerColumns(record []string) (columns, bool) {
	cols := columns{email: -1, first: -1, last: -1}
	for i, cell := range record {
		switch strings.ToLower(strings.TrimSpace(cell)) {
		case "email":
			cols.email = i
		case "first_name":
			cols.first = i
		case "last_name":
			cols.last = i
		}
	}
	return cols, cols.email >= 0
}

func field(record []string, i int) string {
	if i < 0 || i >= len(record) {
		return ""
	}
	return record[i]
}
