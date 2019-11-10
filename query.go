package sqlxx

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Get ...
func (a *Accessor) Get(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	if err := a.validate(); err != nil {
		return err
	}
	return a.build(ctx).querent.Get(dest, query, args...)
}

// Select ...
func (a *Accessor) Select(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	if err := a.validate(); err != nil {
		return err
	}
	return a.build(ctx).querent.Select(dest, query, args...)
}

// Exec ...
func (a *Accessor) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return a.build(ctx).querent.Exec(query, args...)
}

// NamedExec ...
func (a *Accessor) NamedExec(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return a.build(ctx).querent.NamedExec(query, arg)
}

// Query ...
func (a *Accessor) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return a.build(ctx).querent.Query(query, args...)
}

// TxFunc ...
type TxFunc func(context.Context) (interface{}, error)

// RunInTx ...
func (a *Accessor) RunInTx(ctx context.Context, txFn TxFunc) (val interface{}, err, rlbkErr error) {
	if err := a.validate(); err != nil {
		return nil, err, nil
	}

	var tx *sqlx.Tx
	switch q := a.build(ctx).querent.(type) {
	case *sqlx.DB:
		if q == nil {
			return nil, fmt.Errorf("sqlxx: nil dbx"), nil
		}
		tx, err = q.Beginx()
		if err != nil {
			return nil, err, nil
		}
	case *sqlx.Tx:
		return nil, fmt.Errorf("sqlxx: nested transaction"), nil
	default:
		return nil, fmt.Errorf("sqlxx: invalid querent"), nil
	}

	txCtx, err := set(ctx, newAccessor(tx))
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
