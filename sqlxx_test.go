package sqlxx

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
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

	db = New(dbx)

	os.Exit(m.Run())
}

func TestConnect(t *testing.T) {
	db := New(dbx)
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to Connect: %v", err)
	}
}

func TestContext(t *testing.T) {
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Commit()

	ctx := context.Background()
	txCtx := newTxCtx(ctx, tx)

	var q Queryer

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
	testDB(t, db)
}

func testDB(t *testing.T, db *DB) {
	t.Helper()

	var (
		q   string
		err error
		ctx context.Context = context.Background()
	)

	// --------------------
	// Exec
	// --------------------
	q = `INSERT INTO user (email, password) VALUES (?, ?);`
	_, err = db.Exec(ctx, q, "111@example.com", "Passw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, q, "111@example.com", "Passw0rd!") // duplicate entry key
	if err == nil {
		t.Fatalf("want non-nil error")
	}

	// --------------------
	// NamedExec
	// --------------------
	u := User{Email: "222@example.com", Password: "Passw0rd!"}
	q = `INSERT INTO user (email, password) VALUES (:email, :password);`
	_, err = db.NamedExec(ctx, q, u)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.NamedExec(ctx, q, u) // duplicate entry key
	if err == nil {
		t.Fatalf("want non-nil error")
	}

	// --------------------
	// Get
	// --------------------
	q = "SELECT id, email, password FROM user WHERE email = ?;"
	var user User
	err = db.Get(ctx, &user, q, "111@example.com")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%#v", user)
	if got, want := user.Email, "111@example.com"; got != want {
		t.Fatalf("wrong email: got %v, want %v", got, want)
	}
	if got, want := user.Password, "Passw0rd!"; got != want {
		t.Fatalf("wrong password: got %v, want %v", got, want)
	}

	err = db.Get(ctx, &user, q, "123456789@example.com")
	if err == nil {
		t.Fatalf("want non-nil error")
	} else {
		t.Log(err)
	}

	// --------------------
	// Select
	// --------------------
	q = `SELECT id, email, password FROM user;`
	var users []User
	err = db.Select(ctx, &users, q)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%#v", users)
	if got, want := len(users), 2; got != want {
		t.Fatalf("wrong len: got %v, want %v", got, want)
	}
	if got, want := users[0].Email, "111@example.com"; got != want {
		t.Fatalf("wrong email: got %v, want %v", got, want)
	}
	if got, want := users[0].Password, "Passw0rd!"; got != want {
		t.Fatalf("wrong password: got %v, want %v", got, want)
	}
	if got, want := users[1].Email, "222@example.com"; got != want {
		t.Fatalf("wrong email: got %v, want %v", got, want)
	}
	if got, want := users[1].Password, "Passw0rd!"; got != want {
		t.Fatalf("wrong password: got %v, want %v", got, want)
	}

	// --------------------
	// Query
	// --------------------
	// TODO

	// --------------------
	// RunInTx 1
	// --------------------
	var txFn TxFunc
	txSession := Session{ID: "tx-transaction-test", User: User{Email: "333@example.com"}}

	txFn = func(ctx context.Context) error {
		res, err := db.NamedExec(ctx, "INSERT INTO user (email) VALUES (:email);", txSession.User)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		txSession.User.ID = id
		_, err = db.NamedExec(ctx, "INSERT INTO session (id, user_id) VALUES (:id, :user.id)", txSession)
		return err
	}

	err, rbErr := db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var txResult Session
	q = `
  SELECT
    s.id AS id,
    u.id AS 'user.id',
    u.email AS 'user.email'
  FROM session AS s
  INNER JOIN user AS u ON u.id = s.user_id
  WHERE s.id = ?;
  `
	err = db.Get(ctx, &txResult, q, txSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	// if diff := cmp.Diff(txSession, txResult); diff != "" {
	// 	t.Fatalf("wrong result: \n%s", diff)
	// }

	// --------------------
	// RunInTx 2
	// --------------------
	txSession = Session{ID: "tx-transaction-test", User: User{Email: "444@example.com"}}
	err, rbErr = db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal(err)
	} else {
		t.Log(err)
	}
	var txUser User
	err = db.Get(ctx, &txUser, "SELECT id, email FROM user WHERE email = ?;", txSession.User.Email)
	if err == nil {
		t.Fatalf("want non-nil error")
	}

	// --------------------
	// RunInTx 3
	// --------------------
	txSession = Session{ID: "tx-transaction-test-2", User: User{Email: "444@example.com"}}
	txFn = func(ctx context.Context) error {
		res, err := db.NamedExec(ctx, "INSERT INTO user (email) VALUES (:email);", txSession.User)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		txSession.User.ID = id
		_, err = db.NamedExec(ctx, "INSERT INTO session (id, user_id) VALUES (:id, :user.id)", txSession)
		fmt.Println([]string{"0", "1"}[2])
		return err
	}
	err, rbErr = db.RunInTx(ctx, txFn)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal(err)
	} else {
		t.Log(err)
	}
	err = db.Get(ctx, &txUser, "SELECT id, email FROM user WHERE email = ?;", txSession.User.Email)
	if err == nil {
		t.Fatalf("want non-nil error")
	}

	// --------------------
	// RunInTx 4
	// --------------------
	txFn2 := func(ctx context.Context) error {
		err, _ := db.RunInTx(ctx, txFn)
		return err
	}
	err, rbErr = db.RunInTx(ctx, txFn2)
	if rbErr != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal(err)
	} else {
		t.Log(err)
	}
}
