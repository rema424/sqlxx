package sqlxx

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/VividCortex/mysqlerr"
	"github.com/go-sql-driver/mysql"
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
		{NewLogger(os.Stdout), &Option{50 * time.Millisecond, 200, true}, 50 * time.Millisecond, 200, true},
	}

	for i, tt := range tests {
		db := New(dbx, tt.l, tt.opts)

		if err := db.dbx.Ping(); err != nil {
			t.Fatalf("[#%d] failed to Connect: %v", i, err)
		}
		if db.logger == nil {
			t.Errorf("[#%d] want non-nil Logger", i)
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
	defer tx.Commit()

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

func isMysqlErrDupEntry(err error) bool {
	if driverErr, ok := err.(*mysql.MySQLError); ok {
		return driverErr.Number == mysqlerr.ER_DUP_ENTRY // 1062
	}
	return false
}
