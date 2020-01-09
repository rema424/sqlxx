package sqlxx

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/xerrors"
)

var (
	ErrNestedTransaction = xerrors.New("sqlxx: nested transaction")
	ErrNoLogger          = xerrors.New("sqlxx: no logger")
)

type DB struct {
	dbx          *sqlx.DB
	logger       Logger
	slowDuration time.Duration
	warnRows     int
	hideParams   bool
}

const (
	DefaultSlowDuration = 150 * time.Millisecond
	DefaultWarnRows     = 1000
	DefaultHideParams   = false
)

type Option struct {
	SlowDuration time.Duration
	WarnRows     int
	HideParams   bool
}

func New(db *sqlx.DB, l Logger, opts *Option) *DB {
	var (
		slowDuration time.Duration
		warnRows     int
		hideParams   bool
	)

	if l == nil {
		l = NewLogger(os.Stdout)
	}

	if opts != nil {
		slowDuration = opts.SlowDuration
		warnRows = opts.WarnRows
		hideParams = opts.HideParams
	} else {
		slowDuration = DefaultSlowDuration
		warnRows = DefaultWarnRows
		hideParams = DefaultHideParams
	}

	return &DB{db, l, slowDuration, warnRows, hideParams}
}

type ctxKey string

const (
	txCtxKey ctxKey = "tx-ctx-key"
)

type queryer interface {
	QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error)
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
}

func (db *DB) build(ctx context.Context) queryer {
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
	start := time.Now()
	res, err := db.build(ctx).NamedExecContext(ctx, query, arg)
	_, args, _ := sqlx.BindNamed(sqlx.NAMED, query, arg)
	db.log(ctx, query, args, err, countRows(res), time.Since(start))
	return res, err
}

func (db *DB) log(ctx context.Context, query string, args []interface{}, err error, rows int, d time.Duration) {
	if db.logger == nil {
		return
	}

	fn := db.loggerFunc(err, rows, d)
	msg := db.makeLogMsg(query, args, rows, err, d)

	fn(ctx, msg)
}

func (db *DB) loggerFunc(err error, rows int, d time.Duration) loggerFunc {
	if db.logger == nil {
		return nil
	}

	if err != nil && err != sql.ErrNoRows {
		return db.logger.Warnf
	} else if rows > db.warnRows {
		return db.logger.Warnf
	} else if d > db.slowDuration {
		return db.logger.Warnf
	}

	return db.logger.Debugf
}

func (db *DB) makeLogMsg(query string, args []interface{}, rows int, err error, elapsed time.Duration) string {
	var b strings.Builder
	b.Grow(1024)

	if err != nil && err != sql.ErrNoRows {
		b.WriteString(err.Error())
		b.WriteString(" ")
	}

	fmt.Fprintf(&b, "[%.2f ms] [%d rows] ", toMillisec(elapsed), rows)

	b.WriteString(query)

	if !db.hideParams {
		b.WriteString(" ")
		writeArgs(&b, args)
	}

	return b.String()
}

func (db *DB) Secret() *DB {
	clone := db.clone()
	clone.hideParams = true
	return clone
}

func (db *DB) clone() *DB {
	cloneDB := *db
	return &cloneDB
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
