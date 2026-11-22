package segments

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxRules = 20

var dataKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
var eventName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$`)

type Rule struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
	Days  int    `json:"days,omitempty"`
	Rules []Rule `json:"rules,omitempty"`
}

const maxGroupRules = 10

func (r Rule) group(b *builder) (string, error) {
	if (r.Op != "all" && r.Op != "any") || len(r.Rules) == 0 || len(r.Rules) > maxGroupRules {
		return "", fmt.Errorf("invalid_group")
	}
	parts := make([]string, 0, len(r.Rules))
	for _, inner := range r.Rules {
		if inner.Field == "group" {
			return "", fmt.Errorf("invalid_group")
		}
		s, err := inner.sql(b)
		if err != nil {
			return "", err
		}
		parts = append(parts, "("+s+")")
	}
	join := " AND "
	if r.Op == "any" {
		join = " OR "
	}
	return "(" + strings.Join(parts, join) + ")", nil
}

var textColumns = map[string]string{"email": "c.email", "first_name": "c.first_name", "last_name": "c.last_name"}
var dateColumns = map[string]string{"created_at": "c.created_at", "last_engaged_at": "c.last_engaged_at"}
var statuses = map[string]bool{"active": true, "unsubscribed": true, "bounced": true, "complained": true, "invalid": true}

type builder struct {
	args []any
}

func (b *builder) arg(v any) string {
	b.args = append(b.args, v)
	return "$" + strconv.Itoa(len(b.args))
}

func textCondition(col, op, value string, b *builder) (string, error) {
	switch op {
	case "equals":
		return "lower(" + col + ") = lower(" + b.arg(value) + ")", nil
	case "not_equals":
		return "lower(" + col + ") <> lower(" + b.arg(value) + ")", nil
	case "contains":
		return "strpos(lower(" + col + "), lower(" + b.arg(value) + ")) > 0", nil
	case "not_contains":
		return "strpos(lower(" + col + "), lower(" + b.arg(value) + ")) = 0", nil
	case "is_set":
		return "COALESCE(" + col + ", '') <> ''", nil
	case "is_not_set":
		return "COALESCE(" + col + ", '') = ''", nil
	}
	return "", fmt.Errorf("invalid_op")
}

func days(r Rule) (int, error) {
	if r.Days < 1 || r.Days > 3650 {
		return 0, fmt.Errorf("invalid_days")
	}
	return r.Days, nil
}

func (r Rule) sql(b *builder) (string, error) {
	if r.Field == "group" {
		return r.group(b)
	}
	if len(r.Value) > 500 {
		return "", fmt.Errorf("invalid_value")
	}
	if col, ok := textColumns[r.Field]; ok {
		return textCondition(col, r.Op, r.Value, b)
	}
	if col, ok := dateColumns[r.Field]; ok {
		switch r.Op {
		case "within_days", "older_than_days":
			d, err := days(r)
			if err != nil {
				return "", err
			}
			cmp := ">="
			if r.Op == "older_than_days" {
				cmp = "<"
			}
			return col + " " + cmp + " now() - make_interval(days => " + b.arg(d) + ")", nil
		case "before", "after":
			t, err := time.Parse("2006-01-02", r.Value)
			if err != nil {
				return "", fmt.Errorf("invalid_value")
			}
			cmp := "<"
			if r.Op == "after" {
				cmp = ">="
			}
			return col + " " + cmp + " " + b.arg(t), nil
		case "is_set":
			return col + " IS NOT NULL", nil
		case "is_not_set":
			return col + " IS NULL", nil
		}
		return "", fmt.Errorf("invalid_op")
	}
	switch {
	case r.Field == "status":
		if !statuses[r.Value] {
			return "", fmt.Errorf("invalid_value")
		}
		switch r.Op {
		case "equals":
			return "c.status = " + b.arg(r.Value), nil
		case "not_equals":
			return "c.status <> " + b.arg(r.Value), nil
		}
		return "", fmt.Errorf("invalid_op")
	case r.Field == "tag":
		exists := "EXISTS (SELECT 1 FROM contact_tags ct JOIN tags t ON t.id = ct.tag_id WHERE ct.contact_id = c.id AND lower(t.name) = lower(" + b.arg(r.Value) + "))"
		switch r.Op {
		case "has":
			return exists, nil
		case "not_has":
			return "NOT " + exists, nil
		}
		return "", fmt.Errorf("invalid_op")
	case r.Field == "event":
		if !eventName.MatchString(r.Value) {
			return "", fmt.Errorf("invalid_value")
		}
		window := ""
		if r.Days > 0 {
			d, err := days(r)
			if err != nil {
				return "", err
			}
			window = " AND e.created_at >= now() - make_interval(days => " + b.arg(d) + ")"
		}
		exists := "EXISTS (SELECT 1 FROM contact_events e WHERE e.contact_id = c.id AND e.name = " + b.arg(r.Value) + window + ")"
		switch r.Op {
		case "performed":
			return exists, nil
		case "not_performed":
			return "NOT " + exists, nil
		}
		return "", fmt.Errorf("invalid_op")
	case strings.HasPrefix(r.Field, "data."):
		key := strings.TrimPrefix(r.Field, "data.")
		if !dataKey.MatchString(key) {
			return "", fmt.Errorf("invalid_field")
		}
		col := "(c.data ->> " + b.arg(key) + ")"
		switch r.Op {
		case "greater_than", "less_than":
			n, err := strconv.ParseFloat(r.Value, 64)
			if err != nil {
				return "", fmt.Errorf("invalid_value")
			}
			cmp := ">"
			if r.Op == "less_than" {
				cmp = "<"
			}
			return "(CASE WHEN " + col + " ~ '^-?[0-9]+(\\.[0-9]+)?$' THEN " + col + "::numeric " + cmp + " " + b.arg(n) + " ELSE false END)", nil
		}
		return textCondition(col, r.Op, r.Value, b)
	}
	return "", fmt.Errorf("invalid_field")
}

func Compile(match string, rules []Rule, firstArg int) (string, []any, error) {
	if match != "all" && match != "any" {
		return "", nil, fmt.Errorf("invalid_match")
	}
	if len(rules) > maxRules {
		return "", nil, fmt.Errorf("too_many_rules")
	}
	if len(rules) == 0 {
		return "true", nil, nil
	}
	b := &builder{args: make([]any, firstArg)}
	parts := make([]string, 0, len(rules))
	for _, r := range rules {
		s, err := r.sql(b)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, "("+s+")")
	}
	join := " AND "
	if match == "any" {
		join = " OR "
	}
	return "(" + strings.Join(parts, join) + ")", b.args[firstArg:], nil
}
