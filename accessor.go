package sqlxx

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Accessor ...
type Accessor struct {
	querent querent // *sqlx.DB or *sqlx.Tx
}

type querent interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
	NamedExec(query string, arg interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

func newAccessor(q querent) *Accessor {
	return &Accessor{q}
}

// Open ...
func Open(dbx *sqlx.DB) (*Accessor, error) {
	if err := dbx.Ping(); err != nil {
		return nil, err
	}
	return newAccessor(dbx), nil
}

// Close ...
func (a *Accessor) Close() error {
	if err := a.validate(); err != nil {
		return err
	}

	dbx, ok := a.querent.(*sqlx.DB)
	if !ok || dbx == nil {
		return fmt.Errorf("sqlxx: invalid dbx")
	}

	if err := dbx.Close(); err != nil {
		return err
	}

	return nil
}

type ctxValKey string

const (
	accessorKey ctxValKey = "accessor-key"
)

func set(ctx context.Context, a *Accessor) (context.Context, error) {
	if err := a.validate(); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, accessorKey, a), nil
}

func (a *Accessor) build(ctx context.Context) *Accessor {
	if val, ok := ctx.Value(accessorKey).(*Accessor); ok {
		return val
	}
	return a
}

func (a *Accessor) validate() error {
	if a == nil {
		return fmt.Errorf("sqlxx: nil accessor")
	}
	if a.querent == nil {
		return fmt.Errorf("sqlxx: nil querent")
	}
	return nil
}
