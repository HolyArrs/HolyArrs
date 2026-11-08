package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// TimeLayout is how times are stored: UTC with fixed-width nanoseconds, so
// text order is time order.
const TimeLayout = "2006-01-02T15:04:05.000000000Z"

// ErrNotFound is returned by Get when no row has the id.
var ErrNotFound = errors.New("not found")

var (
	colCache sync.Map // reflect.Type -> []column
	timeType = reflect.TypeOf(time.Time{})
)

// Querier is what Get needs: a pool or a transaction.
type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type column struct {
	name  string
	index []int
	json  bool
}

// scanner scans a column into its struct field, decoding JSON and times.
type scanner struct {
	c column
	f reflect.Value
}

// FormatTime formats t for a time column.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeLayout) }

func (s scanner) Scan(src any) error {
	isTime := s.f.Type() == timeType || (s.f.Kind() == reflect.Pointer && s.f.Type().Elem() == timeType)
	if !s.c.json && !isTime {
		if src == nil {
			s.f.SetZero()
			return nil
		}
		target := s.f
		if s.f.Kind() == reflect.Pointer {
			target = reflect.New(s.f.Type().Elem())
			s.f.Set(target)
			target = target.Elem()
		}
		return convert(target, src, s.c.name)
	}
	var text string
	switch v := src.(type) {
	case nil:
		s.f.SetZero()
		return nil
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return fmt.Errorf("column %s: want text, got %T", s.c.name, src)
	}
	if s.c.json {
		s.f.SetZero()
		return json.Unmarshal([]byte(text), s.f.Addr().Interface())
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return fmt.Errorf("column %s: %w", s.c.name, err)
	}
	if s.f.Kind() == reflect.Pointer {
		s.f.Set(reflect.ValueOf(&t))
	} else {
		s.f.Set(reflect.ValueOf(t))
	}
	return nil
}

// Insert adds v (a pointer to a domain record) to table, stamping the who
// and when fields, and sets v's ID.
func Insert(tx *sql.Tx, table string, v any, by Actor) (int64, error) {
	rv, err := structOf(v)
	if err != nil {
		return 0, err
	}
	if m := meta(rv); m != nil {
		now := time.Now().UTC()
		m.CreatedAt, m.UpdatedAt, m.CreatedBy, m.UpdatedBy = now, now, by, by
	}
	var names, marks []string
	var args []any
	for _, c := range columns(rv.Type()) {
		if c.name == "id" {
			continue
		}
		a, err := toDB(c, rv.FieldByIndex(c.index))
		if err != nil {
			return 0, err
		}
		names, marks, args = append(names, c.name), append(marks, "?"), append(args, a)
	}
	q := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", table, strings.Join(names, ", "), strings.Join(marks, ", "))
	res, err := tx.Exec(q, args...)
	if err != nil {
		return 0, fmt.Errorf("insert %s: %w", table, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if m := meta(rv); m != nil {
		m.ID = id
	}
	return id, nil
}

// Update writes every column of v (found by its ID) and stamps updated_*.
func Update(tx *sql.Tx, table string, v any, by Actor) error {
	return UpdateWhere(tx, table, v, by, "")
}

// UpdateWhere is Update that writes only while cond (an SQL condition on
// the row, with its args) still holds; otherwise it returns ErrNotFound and
// changes nothing.
func UpdateWhere(tx *sql.Tx, table string, v any, by Actor, cond string, condArgs ...any) error {
	rv, err := structOf(v)
	if err != nil {
		return err
	}
	m := meta(rv)
	if m == nil || m.ID == 0 {
		return fmt.Errorf("update %s: record has no id", table)
	}
	m.UpdatedAt, m.UpdatedBy = time.Now().UTC(), by
	var sets []string
	var args []any
	for _, c := range columns(rv.Type()) {
		if c.name == "id" || c.name == "created_at" || c.name == "created_by" {
			continue
		}
		a, err := toDB(c, rv.FieldByIndex(c.index))
		if err != nil {
			return err
		}
		sets, args = append(sets, c.name+" = ?"), append(args, a)
	}
	args = append(args, m.ID)
	where := "id = ?"
	if cond != "" {
		where += " AND (" + cond + ")"
		args = append(args, condArgs...)
	}
	res, err := tx.Exec(fmt.Sprintf("UPDATE %s SET %s WHERE %s", table, strings.Join(sets, ", "), where), args...)
	if err != nil {
		return fmt.Errorf("update %s: %w", table, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SelectList returns the column list for v's type, for hand-written queries.
func SelectList(v any) string {
	rv, err := structOf(v)
	if err != nil {
		return ""
	}
	var names []string
	for _, c := range columns(rv.Type()) {
		names = append(names, c.name)
	}
	return strings.Join(names, ", ")
}

// ScanInto returns scan targets for v, in SelectList order.
func ScanInto(v any) []any {
	rv, err := structOf(v)
	if err != nil {
		return nil
	}
	cs := columns(rv.Type())
	out := make([]any, len(cs))
	for i, c := range cs {
		out[i] = scanner{c: c, f: rv.FieldByIndex(c.index)}
	}
	return out
}

// Get reads the row with id from table into dest.
func Get(ctx context.Context, q Querier, table string, id int64, dest any) error {
	cols := SelectList(dest)
	if cols == "" {
		return fmt.Errorf("get %s: want a pointer to a struct, got %T", table, dest)
	}
	err := q.QueryRowContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", cols, table), id).Scan(ScanInto(dest)...)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func columns(t reflect.Type) []column {
	if c, ok := colCache.Load(t); ok {
		return c.([]column)
	}
	var out []column
	var walk func(t reflect.Type, prefix []int)
	walk = func(t reflect.Type, prefix []int) {
		for i := range t.NumField() {
			f := t.Field(i)
			idx := append(append([]int{}, prefix...), i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type, idx)
				continue
			}
			tag := f.Tag.Get("db")
			if tag == "" || tag == "-" {
				continue
			}
			name, opt, _ := strings.Cut(tag, ",")
			out = append(out, column{name: name, index: idx, json: opt == "json"})
		}
	}
	walk(t, nil)
	colCache.Store(t, out)
	return out
}

func structOf(v any) (reflect.Value, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("want a pointer to a struct, got %T", v)
	}
	return rv.Elem(), nil
}

func fmtTime(t time.Time) string { return FormatTime(t) }

func toDB(c column, f reflect.Value) (any, error) {
	if c.json {
		b, err := json.Marshal(f.Interface())
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.name, err)
		}
		return string(b), nil
	}
	switch {
	case f.Type() == timeType:
		return fmtTime(f.Interface().(time.Time)), nil
	case f.Kind() == reflect.Pointer && f.Type().Elem() == timeType:
		if f.IsNil() {
			return nil, nil
		}
		return fmtTime(*f.Interface().(*time.Time)), nil
	case f.Kind() == reflect.Pointer:
		if f.IsNil() {
			return nil, nil
		}
		return f.Elem().Interface(), nil
	case f.Kind() == reflect.String:
		return f.String(), nil
	case f.Kind() == reflect.Bool:
		return f.Bool(), nil
	}
	return f.Interface(), nil
}

func convert(dst reflect.Value, src any, name string) error {
	switch dst.Kind() {
	case reflect.String:
		switch v := src.(type) {
		case string:
			dst.SetString(v)
		case []byte:
			dst.SetString(string(v))
		default:
			return fmt.Errorf("column %s: want text, got %T", name, src)
		}
	case reflect.Bool:
		v, ok := src.(int64)
		if !ok {
			return fmt.Errorf("column %s: want integer for bool, got %T", name, src)
		}
		dst.SetBool(v != 0)
	case reflect.Int, reflect.Int64, reflect.Int32:
		v, ok := src.(int64)
		if !ok {
			return fmt.Errorf("column %s: want integer, got %T", name, src)
		}
		dst.SetInt(v)
	case reflect.Float64:
		switch v := src.(type) {
		case float64:
			dst.SetFloat(v)
		case int64:
			dst.SetFloat(float64(v))
		default:
			return fmt.Errorf("column %s: want real, got %T", name, src)
		}
	default:
		return fmt.Errorf("column %s: unsupported field kind %s", name, dst.Kind())
	}
	return nil
}

func meta(rv reflect.Value) *Meta {
	f := rv.FieldByName("Meta")
	if !f.IsValid() {
		return nil
	}
	m, _ := f.Addr().Interface().(*Meta)
	return m
}
