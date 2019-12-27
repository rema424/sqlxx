package sqlxx

import (
	"context"
	"database/sql"
	"fmt"
	"log"

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

func Connect(db *sqlx.DB) *DB {
	if err := db.Ping(); err != nil {
		log.Fatalln(err)
	}
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

type TxFunc func(context.Context) (interface{}, error)

func (db *DB) RunInTx(ctx context.Context, txFn TxFunc) (val interface{}, err, rlbkErr error) {
	var tx *sqlx.Tx
	switch q := db.build(ctx).(type) {
	case *sqlx.DB:
		if q == nil {
			return nil, ErrNoDB, nil
		}
		tx, err = q.Beginx()
		if err != nil {
			return nil, err, nil
		}
	case *sqlx.Tx:
		return nil, ErrNestedTransaction, nil
	default:
		return nil, ErrInvalidQueryer, nil
	}

	txCtx := newTxCtx(ctx, tx)
	if err != nil {
		return nil, err, nil
	}

	// defer を利用して panic が発生した場合でも recover して rollback を実行する
	defer func() {
		if pnc := recover(); pnc != nil {
			rlbkErr = tx.Rollback()
			if pncErr, ok := pnc.(error); ok {
				err = pncErr
			}
			err = fmt.Errorf("%v", pnc)
		} else if err != nil {
			rlbkErr = tx.Rollback()
		} else if cmtErr := tx.Commit(); cmtErr != nil && cmtErr != sql.ErrTxDone {
			err = cmtErr
		}
	}()

	val, err = txFn(txCtx)
	return
}
