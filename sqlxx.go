package sqlxx

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
	"golang.org/x/xerrors"
)

var (
	ErrNestedTransaction = xerrors.New("sqlxx: nested transaction")
)

type DB struct {
	dbx *sqlx.DB
}

type Queryer interface {
	QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error)
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
}

func New(db *sqlx.DB) *DB {
	return &DB{db}
}

type ctxKey string

const (
	txCtxKey ctxKey = "tx-ctx-key"
)

func (db *DB) build(ctx context.Context) Queryer {
	if tx, ok := ctx.Value(txCtxKey).(*sqlx.Tx); ok {
		return tx
	}
	return db.dbx
}

func newTxCtx(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, txCtxKey, tx)
}

func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	return db.build(ctx).QueryxContext(ctx, query, args...)
}

func (db *DB) Get(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.build(ctx).GetContext(ctx, dest, query, args...)
}

func (db *DB) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.build(ctx).SelectContext(ctx, dest, query, args...)
}

func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.build(ctx).ExecContext(ctx, query, args...)
}

func (db *DB) NamedExec(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	return db.build(ctx).NamedExecContext(ctx, query, arg)
}

type TxFunc func(context.Context) error

func (db *DB) RunInTx(ctx context.Context, txFn TxFunc) (err, rbErr error) {
	var tx *sqlx.Tx
	switch q := db.build(ctx).(type) {
	case *sqlx.DB:
		tx, err = q.Beginx()
		if err != nil {
			return err, nil
		}
	case *sqlx.Tx:
		return ErrNestedTransaction, nil
	}

	defer func() {
		if pnc := recover(); pnc != nil {
			rbErr = tx.Rollback()
			if pncErr, ok := pnc.(error); ok {
				err = pncErr
			} else {
				err = xerrors.Errorf("sqlxx: recovered: %v", pnc)
			}
		} else if err != nil {
			rbErr = tx.Rollback()
		} else if cmtErr := tx.Commit(); cmtErr != nil && cmtErr != sql.ErrTxDone {
			err = cmtErr
		}
	}()

	err = txFn(newTxCtx(ctx, tx))
	return
}
