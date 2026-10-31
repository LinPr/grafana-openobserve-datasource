package openobserve

import (
	"fmt"
	"strconv"
	"strings"
)

// SelectMode describes the shape of a SELECT clause.
type SelectMode int

const (
	// SelectAll is "SELECT * FROM stream".
	SelectAll SelectMode = iota
	// SelectColumns is "SELECT a, b, ... FROM stream".
	SelectColumns
	// SelectAggregate is SELECT with aggregate functions but no explicit
	// aliases, e.g. "SELECT count(*) FROM stream".
	SelectAggregate
)

// ParsedQuery is the lightweight structural view of a user SQL statement that
// the datasource needs in order to shape the response and inject ad-hoc
// filters. It is deliberately not a full SQL parser: OpenObserve runs the SQL
// itself, we only need a handful of top-level facts.
type ParsedQuery struct {
	// Mode is the SELECT shape.
	Mode SelectMode
	// Columns are the projection expressions as written, aliases resolved to
	// the alias when one is present.
	Columns []string
	// Table is the source stream after FROM, without quotes.
	Table string
	// Limit is the top-level LIMIT value, 0 when absent.
	Limit int64
	// Offset is the top-level OFFSET value, 0 when absent.
	Offset int64
	// HasWhere reports whether a top-level WHERE is present.
	HasWhere bool
	// HasGroupBy reports whether the statement aggregates.
	HasGroupBy bool
	// HasOrderBy reports whether an ORDER BY is present.
	HasOrderBy bool
}

// QueryBuilder inspects and rewrites OpenObserve SQL statements.
type QueryBuilder struct{}

// NewQueryBuilder returns a QueryBuilder.
func NewQueryBuilder() *QueryBuilder { return &QueryBuilder{} }

// tokenKind classifies a lexical token.
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokWord
	tokQuotedIdent
	tokString
	tokNumber
	tokPunct
	tokComment
)

type token struct {
	kind  tokenKind
	text  string // lower-cased for words, raw for everything else
	start int
	end   int
	depth int // parenthesis depth at which the token appears
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// tokenizeSQL splits a statement into tokens while tracking parenthesis depth,
// which lets the callers distinguish top-level clauses from subquery ones.
func tokenizeSQL(sql string) []token {
	tokens := make([]token, 0, 64)
	depth := 0
	i := 0
	for i < len(sql) {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			start := i
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			tokens = append(tokens, token{kind: tokComment, text: sql[start:i], start: start, end: i, depth: depth})
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			start := i
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i+1 < len(sql) {
				i += 2
			} else {
				i = len(sql)
			}
			tokens = append(tokens, token{kind: tokComment, text: sql[start:i], start: start, end: i, depth: depth})
		case c == '\'' || c == '"' || c == '`':
			quote := c
			start := i
			i++
			for i < len(sql) {
				if sql[i] == '\\' {
					i += 2
					continue
				}
				if sql[i] == quote {
					// A doubled quote is an escaped quote inside a literal.
					if i+1 < len(sql) && sql[i+1] == quote {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
			if i > len(sql) {
				i = len(sql)
			}
			kind := tokString
			if quote != '\'' {
				kind = tokQuotedIdent
			}
			tokens = append(tokens, token{kind: kind, text: sql[start:i], start: start, end: i, depth: depth})
		case c >= '0' && c <= '9':
			start := i
			for i < len(sql) && (sql[i] >= '0' && sql[i] <= '9' || sql[i] == '.' || sql[i] == 'e' || sql[i] == 'E' || sql[i] == '+' || sql[i] == '-') {
				i++
			}
			tokens = append(tokens, token{kind: tokNumber, text: sql[start:i], start: start, end: i, depth: depth})
		case isIdentStart(c):
			start := i
			for i < len(sql) && isIdentPart(sql[i]) {
				i++
			}
			tokens = append(tokens, token{kind: tokWord, text: strings.ToLower(sql[start:i]), start: start, end: i, depth: depth})
		default:
			switch c {
			case '(':
				tokens = append(tokens, token{kind: tokPunct, text: "(", start: i, end: i + 1, depth: depth})
				depth++
				i++
			case ')':
				depth--
				if depth < 0 {
					depth = 0
				}
				tokens = append(tokens, token{kind: tokPunct, text: ")", start: i, end: i + 1, depth: depth})
				i++
			default:
				tokens = append(tokens, token{kind: tokPunct, text: string(c), start: i, end: i + 1, depth: depth})
				i++
			}
		}
	}
	tokens = append(tokens, token{kind: tokEOF, text: "", start: len(sql), end: len(sql), depth: 0})
	return tokens
}

// meaningful drops comment tokens.
func meaningful(tokens []token) []token {
	out := tokens[:0:0]
	for _, t := range tokens {
		if t.kind == tokComment {
			continue
		}
		out = append(out, t)
	}
	return out
}

// findTopLevel returns the index of the first top-level word token matching
// want, searching forward from index start.
func findTopLevel(tokens []token, start int, want ...string) int {
	for i := start; i < len(tokens); i++ {
		t := tokens[i]
		if t.depth != 0 || t.kind != tokWord {
			continue
		}
		for _, w := range want {
			if t.text == w {
				return i
			}
		}
	}
	return -1
}

// findTopLevelPunct returns the index of the first top-level punct token equal
// to want.
func findTopLevelPunct(tokens []token, start int, want string) int {
	for i := start; i < len(tokens); i++ {
		t := tokens[i]
		if t.depth == 0 && t.kind == tokPunct && t.text == want {
			return i
		}
	}
	return -1
}

// Parse inspects a statement. It returns an error for statements the
// datasource cannot display (anything that is not a SELECT/WITH ... SELECT).
func (qb *QueryBuilder) Parse(raw string) (*ParsedQuery, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty query")
	}
	tokens := meaningful(tokenizeSQL(trimmed))
	// Drop trailing semicolons.
	for len(tokens) > 1 {
		last := tokens[len(tokens)-2]
		if last.kind == tokPunct && last.text == ";" {
			tokens = append(tokens[:len(tokens)-2], tokens[len(tokens)-1])
			continue
		}
		break
	}

	first := 0
	switch tokens[0].text {
	case "select":
	case "with":
		// Skip the CTE definitions: find the top-level SELECT.
		idx := findTopLevel(tokens, 1, "select")
		if idx < 0 {
			return nil, fmt.Errorf("unsupported statement: WITH without a top-level SELECT")
		}
		first = idx
	default:
		return nil, fmt.Errorf("only SELECT statements are supported by this datasource")
	}

	fromIdx := findTopLevel(tokens, first+1, "from")
	if fromIdx < 0 {
		return nil, fmt.Errorf("query has no FROM clause, so no stream could be resolved")
	}

	pq := &ParsedQuery{}
	selectTokens := tokens[first+1 : fromIdx]
	pq.Columns = splitTopLevelExpressions(trimmed, selectTokens)

	star := false
	for _, col := range pq.Columns {
		lower := strings.ToLower(strings.TrimSpace(col))
		if lower == "*" || strings.HasSuffix(lower, ".*") {
			star = true
			break
		}
	}
	switch {
	case star:
		pq.Mode = SelectAll
	case len(pq.Columns) > 0 && isAggregateProjection(pq.Columns):
		pq.Mode = SelectAggregate
	default:
		pq.Mode = SelectColumns
	}

	// Table name: the token right after FROM.
	if fromIdx+1 < len(tokens) {
		tbl := tokens[fromIdx+1]
		if tbl.kind == tokPunct && tbl.text == "(" {
			pq.Table = ""
		} else {
			pq.Table = unquoteIdent(tbl.text)
		}
	}

	if idx := findTopLevel(tokens, fromIdx+1, "where"); idx >= 0 {
		pq.HasWhere = true
	}
	if idx := findTopLevel(tokens, fromIdx+1, "group"); idx >= 0 {
		pq.HasGroupBy = true
	}
	if idx := findTopLevel(tokens, fromIdx+1, "order"); idx >= 0 {
		pq.HasOrderBy = true
	}
	if idx := findTopLevel(tokens, fromIdx+1, "limit"); idx >= 0 {
		if idx+1 < len(tokens) && tokens[idx+1].kind == tokNumber {
			if n, err := strconv.ParseInt(strings.TrimSpace(tokens[idx+1].text), 10, 64); err == nil {
				pq.Limit = n
			}
		}
	}
	if idx := findTopLevel(tokens, fromIdx+1, "offset"); idx >= 0 {
		if idx+1 < len(tokens) && tokens[idx+1].kind == tokNumber {
			if n, err := strconv.ParseInt(strings.TrimSpace(tokens[idx+1].text), 10, 64); err == nil {
				pq.Offset = n
			}
		}
	}
	return pq, nil
}

// isAggregateProjection reports whether every projection is an aggregate call
// such as count(*) or avg(value).
func isAggregateProjection(columns []string) bool {
	aggregates := []string{"count(", "sum(", "avg(", "min(", "max(", "approx_distinct(", "median(", "p50(", "p90(", "p99("}
	for _, col := range columns {
		lower := strings.ToLower(strings.TrimSpace(col))
		if i := strings.Index(lower, " as "); i > 0 {
			lower = strings.TrimSpace(lower[:i])
		}
		matched := false
		for _, agg := range aggregates {
			if strings.HasPrefix(lower, agg) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return len(columns) > 0
}

// splitTopLevelExpressions turns the token slice of a projection into the list
// of expressions, using the original SQL text so quoting is preserved.
func splitTopLevelExpressions(sql string, tokens []token) []string {
	out := make([]string, 0, len(tokens))
	start := -1
	flush := func(end int) {
		if start < 0 {
			return
		}
		expr := strings.TrimSpace(sql[start:end])
		expr = strings.TrimSuffix(expr, ",")
		expr = strings.TrimSpace(expr)
		if expr != "" {
			out = append(out, expr)
		}
		start = -1
	}
	for _, t := range tokens {
		if t.kind == tokPunct && t.text == "," && t.depth == 0 {
			flush(t.start)
			continue
		}
		if start < 0 {
			start = t.start
		}
	}
	if len(tokens) > 0 {
		flush(tokens[len(tokens)-1].end)
	}
	return out
}

func unquoteIdent(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '`' && s[len(s)-1] == '`') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// AdHocOperator is one ad-hoc filter operator supported by the datasource.
type AdHocOperator string

const (
	OpEquals           AdHocOperator = "="
	OpNotEquals        AdHocOperator = "!="
	OpRegexMatch       AdHocOperator = "=~"
	OpRegexNotMatch    AdHocOperator = "!~"
	OpLessThan         AdHocOperator = "<"
	OpLessThanEqual    AdHocOperator = "<="
	OpGreaterThan      AdHocOperator = ">"
	OpGreaterThanEqual AdHocOperator = ">="
	OpIn               AdHocOperator = "IN"
	OpNotIn            AdHocOperator = "NOT IN"
)

// WhereFilter mirrors one Grafana ad-hoc variable filter.
type WhereFilter struct {
	Key       string
	Value     string
	Operation string
}

// sqlOperator maps a Grafana ad-hoc operator onto the OpenObserve SQL dialect.
// OpenObserve supports '~' for regex match and '!~' for regex non-match; the
// '=~'/'!~' forms used by Grafana's UI are not valid there.
func sqlOperator(op string) (string, bool) {
	switch AdHocOperator(strings.ToUpper(strings.TrimSpace(op))) {
	case OpEquals:
		return "=", true
	case OpNotEquals:
		return "<>", true
	case OpRegexMatch:
		return "~", true
	case OpRegexNotMatch:
		return "!~", true
	case OpLessThan:
		return "<", true
	case OpLessThanEqual:
		return "<=", true
	case OpGreaterThan:
		return ">", true
	case OpGreaterThanEqual:
		return ">=", true
	}
	return "", false
}

// QuoteLiteral renders a SQL string literal for the OpenObserve dialect. The
// dialect escapes a single quote by doubling it.
func QuoteLiteral(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// quoteIdent renders an identifier, quoting only when necessary.
func quoteIdent(name string) string {
	if name == "" {
		return name
	}
	plain := true
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(isIdentPart(c) || c == '.') {
			plain = false
			break
		}
	}
	if plain && !isReservedWord(name) {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func isReservedWord(name string) bool {
	switch strings.ToLower(name) {
	case "select", "from", "where", "group", "order", "limit", "by", "and", "or", "not", "in", "as", "on", "join", "having":
		return true
	}
	return false
}

// formatFilterValue renders the right-hand side of an ad-hoc filter. Values
// wrapped in number(...)/string(...) are treated as typed literals, everything
// else becomes a quoted string. "IN" filters accept comma separated lists.
func formatFilterValue(op, rawValue string) (string, error) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return "", fmt.Errorf("empty filter value for operator %s", op)
	}
	if n, ok := numericLiteral(value); ok {
		return n, nil
	}
	if strings.EqualFold(op, string(OpIn)) || strings.EqualFold(op, string(OpNotIn)) {
		parts := splitList(value)
		quoted := make([]string, 0, len(parts))
		for _, p := range parts {
			if n, ok := numericLiteral(p); ok {
				quoted = append(quoted, n)
				continue
			}
			quoted = append(quoted, QuoteLiteral(unquoteValue(p)))
		}
		return "(" + strings.Join(quoted, ", ") + ")", nil
	}
	return QuoteLiteral(unquoteValue(value)), nil
}

// numericLiteral recognises the "number(123)" annotation Grafana sends for
// numeric ad-hoc values, plus plain numeric strings.
func numericLiteral(value string) (string, bool) {
	v := strings.TrimSpace(value)
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "number(") && strings.HasSuffix(v, ")") {
		inner := strings.TrimSpace(v[len("number(") : len(v)-1])
		if _, err := strconv.ParseFloat(inner, 64); err == nil {
			return inner, true
		}
		return "", false
	}
	if strings.HasPrefix(lower, "string(") && strings.HasSuffix(v, ")") {
		return "", false
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return v, true
	}
	return "", false
}

func unquoteValue(v string) string {
	trimmed := strings.TrimSpace(v)
	if len(trimmed) >= 2 {
		if (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'') ||
			(trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') {
			return trimmed[1 : len(trimmed)-1]
		}
	}
	return trimmed
}

// splitList splits a comma separated multi-value filter, honouring quotes.
func splitList(v string) []string {
	var (
		parts []string
		cur   strings.Builder
		quote byte
	)
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
		case c == ',':
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}

// ApplyAdHocFilters injects the ad-hoc variable filters into the statement.
//
// The filters are ANDed with the existing predicate, and an existing OR chain
// is parenthesised first so the injected filters cannot change its meaning.
func (qb *QueryBuilder) ApplyAdHocFilters(raw string, filters []WhereFilter) (string, error) {
	if len(filters) == 0 {
		return raw, nil
	}
	predicates := make([]string, 0, len(filters))
	for _, f := range filters {
		if strings.TrimSpace(f.Key) == "" {
			continue
		}
		op := strings.ToUpper(strings.TrimSpace(f.Operation))
		inList := op == string(OpIn) || op == string(OpNotIn)
		sqlOp, ok := sqlOperator(f.Operation)
		if !ok && !inList {
			return "", fmt.Errorf("unsupported ad-hoc filter operator %q", f.Operation)
		}
		value, err := formatFilterValue(f.Operation, f.Value)
		if err != nil {
			return "", err
		}
		if inList {
			if !strings.HasPrefix(value, "(") {
				value = "(" + value + ")"
			}
			predicates = append(predicates, fmt.Sprintf("%s %s %s", quoteIdent(f.Key), op, value))
			continue
		}
		predicates = append(predicates, fmt.Sprintf("%s %s %s", quoteIdent(f.Key), sqlOp, value))
	}
	if len(predicates) == 0 {
		return raw, nil
	}
	combined := strings.Join(predicates, " AND ")

	trimmed := strings.TrimSpace(raw)
	tokens := meaningful(tokenizeSQL(trimmed))

	// Locate the top-level SELECT first so CTEs are handled.
	anchor := 0
	if len(tokens) > 0 && tokens[0].kind == tokWord && tokens[0].text == "with" {
		idx := findTopLevel(tokens, 1, "select")
		if idx < 0 {
			return "", fmt.Errorf("unsupported statement: WITH without a top-level SELECT")
		}
		anchor = idx
	}
	fromIdx := findTopLevel(tokens, anchor+1, "from")
	if fromIdx < 0 {
		return "", fmt.Errorf("query has no FROM clause, cannot apply ad-hoc filters")
	}

	whereIdx := findTopLevel(tokens, fromIdx+1, "where")
	if whereIdx < 0 {
		// Insert a WHERE clause before GROUP BY / ORDER BY / LIMIT / OFFSET,
		// whichever comes first.
		insertAt := len(trimmed)
		for _, kw := range []string{"group", "order", "limit", "offset", "having"} {
			if idx := findTopLevel(tokens, fromIdx+1, kw); idx >= 0 && tokens[idx].start < insertAt {
				insertAt = tokens[idx].start
			}
		}
		head := strings.TrimRight(trimmed[:insertAt], " \t\r\n")
		tail := trimmed[insertAt:]
		return strings.TrimSpace(head + " WHERE " + combined + " " + tail), nil
	}

	// Existing WHERE: wrap it and append the filter.
	endIdx := len(tokens) - 1 // EOF token
	for _, kw := range []string{"group", "order", "limit", "offset", "having"} {
		if idx := findTopLevel(tokens, whereIdx+1, kw); idx >= 0 && idx < endIdx {
			endIdx = idx
		}
	}
	exprStart := tokens[whereIdx].end
	exprEnd := trimmedEnd(trimmed, tokens, endIdx)
	existing := strings.TrimSpace(trimmed[exprStart:exprEnd])
	if existing == "" {
		return strings.TrimSpace(trimmed[:exprStart] + " " + combined + " " + trimmed[exprEnd:]), nil
	}
	existing = strings.TrimSuffix(existing, ";")
	wrapped := existing
	if topLevelOrExists(tokenizeSQL(existing)) {
		wrapped = "(" + wrapped + ")"
	}
	head := strings.TrimRight(trimmed[:exprStart], " \t\r\n")
	tail := strings.TrimLeft(trimmed[exprEnd:], " \t\r\n")
	rebuilt := head + " " + wrapped + " AND " + combined
	if tail != "" {
		rebuilt += " " + tail
	}
	return rebuilt, nil
}

func trimmedEnd(sql string, tokens []token, idx int) int {
	if idx < 0 || idx >= len(tokens) {
		return len(strings.TrimSpace(sql))
	}
	end := tokens[idx].start
	for end > 0 && (sql[end-1] == ' ' || sql[end-1] == '\n' || sql[end-1] == '\t' || sql[end-1] == '\r') {
		end--
	}
	return end
}

// topLevelOrExists reports whether the expression contains a top-level OR.
func topLevelOrExists(tokens []token) bool {
	for _, t := range tokens {
		if t.depth == 0 && t.kind == tokWord && t.text == "or" {
			return true
		}
	}
	return false
}
