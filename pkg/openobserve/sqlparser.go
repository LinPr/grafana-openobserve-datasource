package openobserve

import (
	"fmt"
	"strings"

	"regexp"

	"github.com/auxten/postgresql-parser/pkg/sql/parser"
	"github.com/auxten/postgresql-parser/pkg/sql/sem/tree"
	"github.com/auxten/postgresql-parser/pkg/walk"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/xwb1989/sqlparser"
)

const (
	SqlSelectALlColumns        = "sql_select_all_columns"
	SqlSelectSpecifiedcColumns = "sql_select_specified_columns"
)

// SqlParser is responsible for parsing SQL queries
type SqlParser struct{}

// NewSqlParser creates a new instance of SqlParser
func NewSqlParser() *SqlParser {
	return &SqlParser{}
}

type SQL struct {
	selectMode     string
	selectColumns  []string
	whereVariables []string
	CompletedSql   string
	Limit          int64 // Extracted LIMIT value from SQL, 0 means no limit specified
}

const (
	Equals        = "="
	NotEqual      = "!="
	MatchRegex    = "=~"
	NotMatchRegex = "!~"
	LessThan      = "<"
	GreaterThan   = ">"
)

// OperatorMap key = adhoc operator, value = postgresql operator
var OperatorMap = map[string]string{
	Equals:        "=",
	NotEqual:      "<>",
	MatchRegex:    "~",
	NotMatchRegex: "!~",
	LessThan:      "<",
	GreaterThan:   ">",
}

// WhereFilter represents a filter condition in the WHERE clause
type WhereFilter struct {
	Key       string
	Value     string
	Operation string
}

func (f *WhereFilter) String() (string, error) {
	sqlOp, ok := OperatorMap[f.Operation]
	if !ok {
		return "", fmt.Errorf("unsupported operation: %s", f.Operation)
	}
	return fmt.Sprintf("%s %s %s", f.Key, sqlOp, f.Value), nil
}

func (f *WhereFilter) IsValueNumber() (string, bool) {
	re := regexp.MustCompile(`^number\((.+)\)$`)
	matches := re.FindStringSubmatch(f.Value)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

// CompeleteSqlWithAdhocFilters completes the SQL with ad-hoc filters
func (sp *SqlParser) CompeleteSqlWithAdhocFilters(rawSql string, filters []WhereFilter) (string, error) {
	if rawSql == "\\dt" {
		return rawSql, nil // no need to parse, just return the original rawSql
	}

	stmt, err := sqlparser.Parse(rawSql)
	if err != nil {
		return "", err
	}
	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok {
		return "", fmt.Errorf("not a SELECT statement: %s", rawSql)
	}

	for _, filter := range filters {
		// Create a new WHERE condition
		sqlOp, ok := OperatorMap[filter.Operation]
		if !ok {
			return "", fmt.Errorf("unsupported operation: %s", filter.Operation)
		}

		whereExpr := &sqlparser.ComparisonExpr{
			Left:     sqlparser.NewValArg([]byte(filter.Key)),
			Operator: sqlOp,
			Right:    sqlparser.NewStrVal([]byte(filter.Value)),
		}

		// is the right value is a number, replace it with a ValArg
		if value, ok := filter.IsValueNumber(); ok {
			whereExpr.Right = sqlparser.NewValArg([]byte(value))
		}

		// if there is already a WHERE condition, append the new condition
		if selectStmt.Where != nil {
			selectStmt.Where = &sqlparser.Where{
				Type: sqlparser.WhereStr,
				Expr: &sqlparser.AndExpr{
					Left:  selectStmt.Where.Expr,
					Right: whereExpr,
				},
			}
		} else {
			// else create a new WHERE condition
			selectStmt.Where = &sqlparser.Where{
				Type: sqlparser.WhereStr,
				Expr: whereExpr,
			}
		}
	}

	return strings.ReplaceAll(sqlparser.String(selectStmt), "`", ""), nil
}

// parseSql parses the SQL string and returns a SQL object with select columns and where variables
func (sp *SqlParser) ParseSql(sqlStr string) (*SQL, error) {
	log.DefaultLogger.Debug("ParseSql called", "sqlStr", sqlStr)

	sqlStr = strings.ReplaceAll(sqlStr, "'", "\"")

	stmts, err := parser.Parse(sqlStr)
	if err != nil {
		return nil, err
	}

	selectedColumns, err := parseSqlSelectColumns(stmts)
	if err != nil {
		return nil, err
	}

	whereVariables, err := parseSqlWhereConditions(stmts)
	if err != nil {
		return nil, err
	}

	// Extract LIMIT value from SQL using sqlparser (more reliable for LIMIT extraction)
	limitValue := extractLimitFromSql(sqlStr)

	if len(selectedColumns) == 1 && selectedColumns[0] == "*" {
		return &SQL{
			selectMode:     SqlSelectALlColumns,
			selectColumns:  selectedColumns,
			whereVariables: whereVariables,
			Limit:          limitValue,
		}, nil
	}

	return &SQL{
		selectMode:     SqlSelectSpecifiedcColumns,
		selectColumns:  selectedColumns,
		whereVariables: whereVariables,
		Limit:          limitValue,
	}, nil
}

// parseSqlSelectColumns extracts the selected columns from the SQL statements
func parseSqlSelectColumns(stmts parser.Statements) ([]string, error) {
	columnNames := make([]string, 0)
	w := &walk.AstWalker{
		Fn: func(ctx interface{}, node interface{}) (stop bool) {
			if selectExprs, ok := node.(tree.SelectExprs); ok {
				for _, expr := range selectExprs {
					if expr.As.String() != "\"\"" {
						columnNames = append(columnNames, expr.As.String())
					} else {
						columnNames = append(columnNames, expr.Expr.String())
					}
				}
				return true
			}
			return false
		},
	}

	if _, err := w.Walk(stmts, nil); err != nil {
		return nil, err
	}
	return columnNames, nil
}

// parseSqlWhereConditions extracts the WHERE conditions from the SQL statements
func parseSqlWhereConditions(stmts parser.Statements) ([]string, error) {
	vist := NewVisitor()
	w := &walk.AstWalker{
		Fn: func(ctx interface{}, node interface{}) (stop bool) {
			if whereExprs, ok := node.(*tree.Where); ok {
				whereExprs.Expr.Walk(vist)
			}
			return false
		},
	}

	if _, err := w.Walk(stmts, nil); err != nil {
		return nil, err
	}
	return vist.variables, nil
}

type Visitor struct {
	variables []string
}

func NewVisitor() *Visitor {
	return &Visitor{
		variables: make([]string, 0),
	}
}
func (v *Visitor) VisitPre(expr tree.Expr) (recurse bool, newExpr tree.Expr) {
	if e, ok := expr.(tree.VariableExpr); ok {
		v.variables = append(v.variables, e.String())
	}
	return true, expr
}

func (v *Visitor) VisitPost(expr tree.Expr) (newNode tree.Expr) {
	// return expr
	return nil
}

// extractLimitFromSql extracts the LIMIT value from SQL string using sqlparser
func extractLimitFromSql(sqlStr string) int64 {
	stmt, err := sqlparser.Parse(sqlStr)
	if err != nil {
		return 0 // Return 0 if parsing fails (means no limit or invalid SQL)
	}

	selectStmt, ok := stmt.(*sqlparser.Select)
	if !ok {
		return 0 // Not a SELECT statement
	}

	if selectStmt.Limit == nil {
		return 0 // No LIMIT clause
	}

	// LIMIT can be: LIMIT n or LIMIT offset, n
	// Rowcount is the second value if both present, or the only value if only one
	if selectStmt.Limit.Rowcount == nil {
		return 0
	}

	// Try to extract numeric value from the expression
	// The Rowcount is an Expr, which could be a number or expression
	// We'll try to convert it to string first, then parse as int
	limitStr := sqlparser.String(selectStmt.Limit.Rowcount)

	// Parse the string as int64
	var limitValue int64
	_, err = fmt.Sscanf(limitStr, "%d", &limitValue)
	if err != nil {
		log.DefaultLogger.Warn("extractLimitFromSql: Failed to parse LIMIT value", "limitStr", limitStr, "error", err)
		return 0
	}

	return limitValue
}
