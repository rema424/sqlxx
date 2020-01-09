package sqlxx

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/go-cmp/cmp"
	"github.com/jmoiron/sqlx"
)

/*
  mysql.server start
  mysql -uroot -e 'create database if not exists sqlxxtest;'
  mysql -uroot -e 'create user if not exists sqlxxtester@localhost identified by "Passw0rd!";'
  mysql -uroot -e 'grant all privileges on sqlxxtest.* to sqlxxtester@localhost;'
  mysql -uroot -e 'show databases;'
  mysql -uroot -e 'select host, user from mysql.user;'
  mysql -uroot -e 'show grants for sqlxxtester@localhost;'
*/

const CreateUser = `
create table if not exists user (
  id bigint not null auto_increment,
  email varchar(255) character set latin1 collate latin1_bin not null default '',
  password varchar(255) not null default '',
  primary key (id),
  unique key (email)
);
`
const DropUser = `
drop table if exists user;
`

const CreateSession = `
create table if not exists session (
  id varchar(255) character set latin1 collate latin1_bin not null default '',
  csrf varchar(255) not null default '',
  user_id bigint not null default 0,
  expire_at bigint not null default 0,
  primary key (id),
  foreign key (user_id) references user (id) on delete cascade on update cascade,
  key (user_id)
);
`
const DropSession = `
drop table if exists session;
`

const testPassword = "Passw0rd!"

var (
	dbx *sqlx.DB
	db  *DB
)

type User struct {
	ID       int64  `db:"id"`
	Email    string `db:"email"`
	Password string `db:"password"`
}

type Session struct {
	ID   string `db:"id"`
	User User   `db:"user"`
}

func newUser(email, password string) User  { return User{Email: email, Password: password} }
func newSession(id string, u User) Session { return Session{id, u} }

func createUser(ctx context.Context, db *DB, s Session) (Session, error) {
	q := `INSERT INTO user (email, password) VALUES (:email, :password);`
	res, err := db.NamedExec(ctx, q, s.User)
	if err != nil {
		return s, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return s, err
	}
	s.User.ID = id
	return s, nil
}

func createSession(ctx context.Context, db *DB, s Session) (Session, error) {
	q := `INSERT INTO session (id, user_id) VALUES (:id, :user.id);`
	_, err := db.NamedExec(ctx, q, s)
	return s, err
}

func getUserByEmail(ctx context.Context, db *DB, email string) (User, error) {
	q := `SELECT id, email, password FROM user WHERE email = ?;`
	var dest User
	err := db.Get(ctx, &dest, q, email)
	return dest, err
}

func getSessionByID(ctx context.Context, db *DB, id string) (Session, error) {
	q := `
  SELECT
    s.id AS id,
    u.id AS 'user.id',
    u.email AS 'user.email',
    u.password AS 'user.password'
  FROM session AS s
  INNER JOIN user AS u ON u.id = s.user_id
  WHERE s.id = ?;
  `
	var dest Session
	err := db.Get(ctx, &dest, q, id)
	return dest, err
}

func TestMain(m *testing.M) {

	var err error
	dbx, err = sqlx.Connect("mysql", "sqlxxtester:Passw0rd!@tcp(127.0.0.1:3306)/sqlxxtest?collation=utf8mb4_bin&interpolateParams=true&parseTime=true&maxAllowedPacket=0")
	if err != nil {
		log.Fatalf("sqlx.Connect: %v", err)
	}
	defer dbx.Close()

	dbx.MustExec(DropSession)
	dbx.MustExec(DropUser)
	dbx.MustExec(CreateUser)
	dbx.MustExec(CreateSession)

	db = New(dbx, nil, nil)

	os.Exit(m.Run())
}

func TestNew(t *testing.T) {
	tests := []struct {
		l     Logger
		opts  *Option
		wantD time.Duration
		wantR int
		wantP bool
	}{
		{nil, nil, DefaultSlowDuration, DefaultWarnRows, DefaultHideParams},
		{nil, &Option{}, 0, 0, false},
		{NewLogger(ioutil.Discard), &Option{50 * time.Millisecond, 200, true}, 50 * time.Millisecond, 200, true},
	}

	for i, tt := range tests {
		db := New(dbx, tt.l, tt.opts)

		if err := db.dbx.Ping(); err != nil {
			t.Fatalf("[#%d] failed to Connect: %v", i, err)
		}
		if got, want := db.slowDuration, tt.wantD; got != want {
			t.Errorf("[#%d] wrong slowDuration: got %v, want %v", i, got, want)
		}
		if got, want := db.warnRows, tt.wantR; got != want {
			t.Errorf("[#%d] wrong warnRows: got %v, want %v", i, got, want)
		}
		if got, want := db.hideParams, tt.wantP; got != want {
			t.Errorf("[#%d] wrong hideParams: got %v, want %v", i, got, want)
		}
	}
}

func TestContext(t *testing.T) {
	tx, err := db.dbx.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Commit(); err != nil {
			t.Error(err)
		}
	}()

	ctx := context.Background()
	txCtx := newTxCtx(ctx, tx)

	var q queryer

	q = db.build(ctx)
	if _, ok := q.(*sqlx.DB); !ok {
		t.Fatalf("want *sqlx.DB, got %T", q)
	}

	q = db.build(txCtx)
	if _, ok := q.(*sqlx.Tx); !ok {
		t.Fatalf("want *sqlx.Tx, got %T", q)
	}
}

func TestClone(t *testing.T) {
	logger := NewLogger(ioutil.Discard)
	opts := &Option{234 * time.Microsecond, 435, true}
	db := New(dbx, logger, opts)
	clone := db.clone()

	if got, want := fmt.Sprintf("%p", clone), fmt.Sprintf("%p", db); got == want {
		t.Errorf("must not be the same address: got %v, want %v", got, want)
	}

	if got, want := *clone, *db; got != want {
		t.Errorf("must be the same field values: got %#v, want %#v", got, want)
	}
}

func TestSecret(t *testing.T) {
	tests := []struct {
		opts *Option
	}{
		{nil},
		{&Option{HideParams: true}},
		{&Option{HideParams: false}},
	}

	for _, tt := range tests {
		db := New(dbx, nil, tt.opts)
		before := db.hideParams
		clone := db.Secret()

		if got, want := fmt.Sprintf("%p", clone), fmt.Sprintf("%p", db); got == want {
			t.Errorf("must not be the same address: got %v, want %v", got, want)
		}
		if !clone.hideParams {
			t.Errorf("hideParams must be 'true'. got %t", clone.hideParams)
		}
		if db.hideParams != before {
			t.Errorf("db.hideParams must not be overwritten. before %t, after %t", before, db.hideParams)
		}
	}
}

func TestLog(t *testing.T) {
	var buf bytes.Buffer

	db := &DB{
		slowDuration: DefaultSlowDuration,
		warnRows:     DefaultWarnRows,
		hideParams:   DefaultHideParams,
		logger:       NewLogger(&buf),
	}

	ctx := context.Background()
	db.log(ctx, "query", []interface{}{"arg"}, errors.New("error"), 10, 100*time.Millisecond)

	got := buf.String()
	parts := strings.Split(got, " ")
	wantP := "[WARN]"
	wantS := "error [100.00 ms] [10 rows] query [arg]\n"
	if !strings.HasPrefix(got, wantP) {
		t.Errorf("Prefix: want %s, got %s", wantP, parts[0])
	}
	if !strings.HasSuffix(got, wantS) {
		t.Errorf("Suffix: want '%s', got '%s'", wantS, strings.Join(parts[3:], " "))
	}
	t.Logf("want %s, got %s", wantP+" "+wantS, got)
}

func TestLogNilLogger(t *testing.T) {
	db := &DB{logger: nil}
	ctx := context.Background()

	defer func() {
		if pnc := recover(); pnc != nil {
			t.Errorf("want no panic, got %v", pnc)
		}
	}()

	db.log(ctx, "query", nil, nil, 0, 0)
}

func TestLoggerFunc(t *testing.T) {
	tests := []struct {
		name         string
		slowDuration time.Duration
		warnRows     int
		d            time.Duration
		rows         int
		err          error
		want         string
	}{
		{
			"normal",
			150 * time.Millisecond,
			1000,
			150 * time.Millisecond,
			1000,
			nil,
			"[DEBUG]",
		},
		{
			"duration",
			150 * time.Millisecond,
			1000,
			151 * time.Millisecond,
			1000,
			nil,
			"[WARN]",
		},
		{
			"rows",
			150 * time.Millisecond,
			1000,
			150 * time.Millisecond,
			1001,
			nil,
			"[WARN]",
		},
		{
			"error",
			150 * time.Millisecond,
			1000,
			150 * time.Millisecond,
			1000,
			errors.New("some error"),
			"[WARN]",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var buf bytes.Buffer
			db := &DB{
				slowDuration: tt.slowDuration,
				warnRows:     tt.warnRows,
				logger:       NewLogger(&buf),
			}

			fn := db.loggerFunc(tt.err, tt.rows, tt.d)
			fn(context.Background(), "some message...")

			got := buf.String()
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("#%d: want %s, got %s", i, tt.want, strings.Split(got, " ")[0])
			}
			t.Logf("#%d: want %s, got %s", i, tt.want, strings.Split(got, " ")[0])
		})
	}
}

func TestLoggerFuncNilLogger(t *testing.T) {
	db := &DB{logger: nil}
	fn := db.loggerFunc(nil, DefaultWarnRows, DefaultSlowDuration)
	if fn != nil {
		t.Errorf("want nil")
	}
}

func TestMakeLogMsg(t *testing.T) {
	tests := []struct {
		query      string
		args       []interface{}
		rows       int
		err        error
		elapsed    time.Duration
		hideParams bool
		want       string
	}{
		{
			"select * from person;",
			[]interface{}{},
			50,
			nil,
			123454 * time.Microsecond,
			false,
			"[123.45 ms] [50 rows] select * from person; []",
		},
		{
			"select * from person where id = ?;",
			[]interface{}{1},
			1,
			nil,
			123454 * time.Microsecond,
			false,
			"[123.45 ms] [1 rows] select * from person where id = ?; [1]",
		},
		{
			"select * from person where id = ?;",
			[]interface{}{1},
			1,
			nil,
			123456 * time.Microsecond,
			false,
			"[123.46 ms] [1 rows] select * from person where id = ?; [1]",
		},
		{
			"select * from person where id = ?;",
			[]interface{}{1},
			1,
			nil,
			123456 * time.Microsecond,
			true,
			"[123.46 ms] [1 rows] select * from person where id = ?;",
		},
		{
			"insert into person (name, email) values (?, ?);",
			[]interface{}{"alice", "alice@example.com"},
			1,
			nil,
			2345678 * time.Microsecond,
			false,
			"[2345.68 ms] [1 rows] insert into person (name, email) values (?, ?); [alice, alice@example.com]",
		},
		{
			"insert into person (name, email) values (?, ?);",
			[]interface{}{"alice", "alice@example.com"},
			1,
			errors.New("sqlxx: some error occurs"),
			2345678 * time.Microsecond,
			false,
			"sqlxx: some error occurs [2345.68 ms] [1 rows] insert into person (name, email) values (?, ?); [alice, alice@example.com]",
		},
	}

	for i, tt := range tests {
		db := &DB{hideParams: tt.hideParams}
		got := db.makeLogMsg(tt.query, tt.args, tt.rows, tt.err, tt.elapsed)

		if got != tt.want {
			t.Errorf("#%d: want %s, got %s", i, tt.want, got)
		}
		// t.Logf("#%d: want %s, got %s", i, tt.want, got)
	}
}

func TestMySQL(t *testing.T) {
	ctx := context.Background()
	testExecMySQL(ctx, db, t)
	testNamedExecMySQL(ctx, db, t)
	testGetMySQL(ctx, db, t)
	testSelectMySQL(ctx, db, t)
	testQueryMySQL(ctx, db, t)
	testRunInTxSuccessMySQL(ctx, db, t)
	testRunInTxErrorMySQL(ctx, db, t)
	testRunInTxRuntimePanicMySQL(ctx, db, t)
	testRunInTxManualPanicMySQL(ctx, db, t)
	testRunInTxNestMySQL(ctx, db, t)
}

func testExecMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	u := newUser("exec@example.com", testPassword)
	q := "INSERT INTO user (email, password) VALUES (?, ?);"
	_, err := db.Exec(ctx, q, u.Email, u.Password)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, q, u.Email, u.Password) // duplicate entry
	if err == nil {
		t.Fatalf("want non-nil error")
	}
}

func testNamedExecMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	u := newUser("namedExec@example.com", testPassword)
	q := `INSERT INTO user (email, password) VALUES (:email, :password);`
	_, err := db.NamedExec(ctx, q, u)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.NamedExec(ctx, q, u) // duplicate entry
	if err == nil {
		t.Fatalf("want non-nil error")
	}
}

func testGetMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	u := newUser("exec@example.com", testPassword)
	q := "SELECT id, email, password FROM user WHERE email = ?;"
	var got User
	err := db.Get(ctx, &got, q, u.Email)
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != u.Email {
		t.Fatalf("wrong email: got %v, want %v", got.Email, u.Email)
	}
	if got.Password != u.Password {
		t.Fatalf("wrong password: got %v, want %v", got.Password, u.Password)
	}
	err = db.Get(ctx, &got, q, "123456789@example.com")
	if err == nil {
		t.Fatalf("want non-nil error")
	}
}

func testSelectMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	q := `SELECT id, email, password FROM user;`
	var got []User
	err := db.Select(ctx, &got, q)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(got), 2; got != want {
		t.Fatalf("wrong len: got %v, want %v", got, want)
	}
	if got, want := got[0].Email, "exec@example.com"; got != want {
		t.Fatalf("wrong email: got %v, want %v", got, want)
	}
	if got, want := got[0].Password, testPassword; got != want {
		t.Fatalf("wrong password: got %v, want %v", got, want)
	}
	if got, want := got[1].Email, "namedExec@example.com"; got != want {
		t.Fatalf("wrong email: got %v, want %v", got, want)
	}
	if got, want := got[1].Password, testPassword; got != want {
		t.Fatalf("wrong password: got %v, want %v", got, want)
	}
}

func testQueryMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	q := `SELECT id, email, password FROM user;`
	rows, err := db.Query(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		var u User
		err := rows.StructScan(&u)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := u.Email, "exec@example.com"; got != want {
			t.Fatalf("wrong email: got %v, want %v", got, want)
		}
		if got, want := u.Password, testPassword; got != want {
			t.Fatalf("wrong password: got %v, want %v", got, want)
		}
	}
	if rows.Next() {
		var u User
		err := rows.StructScan(&u)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := u.Email, "namedExec@example.com"; got != want {
			t.Fatalf("wrong email: got %v, want %v", got, want)
		}
		if got, want := u.Password, testPassword; got != want {
			t.Fatalf("wrong password: got %v, want %v", got, want)
		}
	}
	if rows.Next() {
		t.Fatalf("want false")
	}
}

func testRunInTxSuccessMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	s := newSession("tx-success", newUser("tx-success@example.com", testPassword))
	txFn := func(ctx context.Context) error {
		var err error
		s, err = createUser(ctx, db, s)
		if err != nil {
			return err
		}
		s, err = createSession(ctx, db, s)
		return err
	}

	err, rbErr := db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}

	got, err := getSessionByID(ctx, db, s.ID)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(got, s); diff != "" {
		t.Fatalf("wrong result: \n%s", diff)
	}
}

func testRunInTxErrorMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	s := newSession("tx-success", newUser("tx-error@example.com", testPassword))
	txFn := func(ctx context.Context) error {
		var err error
		s, err = createUser(ctx, db, s) // success
		if err != nil {
			return err
		}
		s, err = createSession(ctx, db, s) // duplicate entry key 'tx-success'
		return err
	}

	err, rbErr := db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("want non-nil error")
	}

	_, err = getUserByEmail(ctx, db, s.User.Email)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}
}

func testRunInTxRuntimePanicMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	s := newSession("tx-runtime-panic", newUser("tx-runtime-panic@example.com", testPassword))
	txFn := func(ctx context.Context) error {
		var err error
		s, err = createUser(ctx, db, s) // success
		if err != nil {
			return err
		}
		s, err = createSession(ctx, db, s) // success
		if err != nil {
			return err
		}
		fmt.Println([]string{}[99]) // runtime panic and rollback
		return nil
	}

	err, rbErr := db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("want non-nil error")
	} else if _, ok := err.(runtime.Error); !ok {
		t.Fatal(err)
	}

	_, err = getUserByEmail(ctx, db, s.User.Email)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}

	_, err = getSessionByID(ctx, db, s.ID)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}
}

func testRunInTxManualPanicMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	s := newSession("tx-manual-panic", newUser("tx-manual-panic@example.com", testPassword))
	txFn := func(ctx context.Context) error {
		var err error
		s, err = createUser(ctx, db, s) // success
		if err != nil {
			return err
		}
		s, err = createSession(ctx, db, s) // success
		if err != nil {
			return err
		}
		defer panic("manual panic!!!") // manual panic and rollback
		return nil
	}

	err, rbErr := db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("want non-nil error")
	}

	_, err = getUserByEmail(ctx, db, s.User.Email)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}

	_, err = getSessionByID(ctx, db, s.ID)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}
}

func testRunInTxNestMySQL(ctx context.Context, db *DB, t *testing.T) {
	t.Helper()

	s := newSession("tx-nest", newUser("tx-nest@example.com", testPassword))
	txFn := func(ctx context.Context) error {
		txFn := func(ctx context.Context) error {
			var err error
			s, err = createUser(ctx, db, s) // success
			if err != nil {
				return err
			}
			s, err = createSession(ctx, db, s) // success
			return err
		}
		err, _ := db.RunInTx(ctx, txFn) // nest
		return err
	}

	err, rbErr := db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != ErrNestedTransaction {
		t.Fatal("want nested transaction error")
	}

	_, err = getUserByEmail(ctx, db, s.User.Email)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}

	_, err = getSessionByID(ctx, db, s.ID)
	if err == nil {
		t.Fatal("want non-nil error")
	} else if err != sql.ErrNoRows {
		t.Fatal("want sql.ErrNoRows")
	}
}

// func isMysqlErrDupEntry(err error) bool {
// 	if driverErr, ok := err.(*mysql.MySQLError); ok {
// 		return driverErr.Number == mysqlerr.ER_DUP_ENTRY // 1062
// 	}
// 	return false
// }
