package sqlxx

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"golang.org/x/xerrors"
)

var (
	ErrNoDB              = xerrors.New("sqlxx: no db")
	ErrInvalidQueryer    = xerrors.New("sqlxx: invalid queryer")
	ErrNestedTransaction = xerrors.New("sqlxx: nested transaction")
)

type DB struct {
	*sqlx.DB
}

type Queryer interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Select(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
	NamedExec(query string, arg interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
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
	return db.DB
}

func newTxCtx(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, txCtxKey, tx)
}

func (db *DB) Get(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.build(ctx).Get(dest, query, args...)
}

func (db *DB) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return db.build(ctx).Select(dest, query, args...)
}

func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.build(ctx).Exec(query, args...)
}

func (db *DB) NamedExec(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	return db.build(ctx).NamedExec(query, arg)
}

func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.build(ctx).Query(query, args...)
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
	default:
		return ErrInvalidQueryer, nil
	}

	defer func() {
		if pnc := recover(); pnc != nil {
			rbErr = tx.Rollback()
			if pncErr, ok := pnc.(error); ok {
				err = pncErr
			}
			err = fmt.Errorf("%v", pnc)
		} else if err != nil {
			rbErr = tx.Rollback()
		} else if cmtErr := tx.Commit(); cmtErr != nil && cmtErr != sql.ErrTxDone {
			err = cmtErr
		}
	}()

	err = txFn(newTxCtx(ctx, tx))
	return
}
