# sqlxx

[![Go Report Card](https://goreportcard.com/badge/github.com/rema424/sqlxx)](https://goreportcard.com/report/github.com/rema424/sqlxx)
[![codebeat badge](https://codebeat.co/badges/9c1ae3a7-2945-4ec9-823c-f0a13e401a6b)](https://codebeat.co/projects/github-com-rema424-sqlxx-master)
[![Coverage Status](https://coveralls.io/repos/github/rema424/sqlxx/badge.svg?branch=master)](https://coveralls.io/github/rema424/sqlxx?branch=master)
[![Build Status](https://travis-ci.org/rema424/sqlxx.svg?branch=master)](https://travis-ci.org/rema424/sqlxx)
[![GoDoc](https://godoc.org/github.com/rema424/sqlxx?status.svg)](https://godoc.org/github.com/rema424/sqlxx)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

[jmoiron/sqlx](https://github.com/jmoiron/sqlx) を拡張してトランザクション管理をアプリケーションコアから切り離せるようにしたライブラリ

## Sample Usage

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rema424/sqlxx"
	"golang.org/x/crypto/bcrypt"
)

// signup situation

const sessionSchema = `
create table if not exists session (
  id varchar(255) character set latin1 collate latin1_bin not null default '',
  expire_at bigint not null default 0,
  user_id bigint not null default 0,
  primary key (id),
  foreign key (user_id) references user (id) on delete cascade on update cascade,
  key (user_id)
);
`

const userSchema = `
create table if not exists user (
  id bigint not null auto_increment,
  email varchar(255) character set latin1 collate latin1_bin not null default '',
  password varchar(255) not null default '',
  primary key (id),
  unique key (email)
);
`

func main() {
	// create database, user
	//
	// $ mysql.server start
	// $ mysql -uroot -e 'create database if not exists sqlxxtest;'
	// $ mysql -uroot -e 'create user if not exists sqlxxtester@localhost identified by "Passw0rd!";'
	// $ mysql -uroot -e 'grant all privileges on sqlxxtest.* to sqlxxtester@localhost;'
	//

	// init *sqlx.DB
	dbx, err := sqlx.Connect("mysql", "sqlxxtester:Passw0rd!@tcp(127.0.0.1:3306)/sqlxxtest?collation=utf8mb4_bin&interpolateParams=true&parseTime=true&maxAllowedPacket=0")
	if err != nil {
		log.Fatalln(err)
	}
	defer dbx.Close()

	// create tables
	dbx.MustExec(userSchema)
	dbx.MustExec(sessionSchema)

	// init *sqlxx.DB
	db := sqlxx.New(dbx, sqlxx.NewLogger(os.Stdout), nil)

	// dependency injection
	it := NewInteractor(NewRepositoryImpl(db))

	// exec signup
	ctx := context.Background()
	s, err := it.Signup(ctx, "alice@example.com", "Passw0rd!")
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Printf("%+v\n", s)
	// -> {ID:0a85a9d7-d2cc-4650-b881-6c94ff7ea9e0 ExpireAt:1580791243 User:{ID:11 Email:alice@example.com Password:$2a$10$gbfMBPwX9FWXnMeHQhlMcOrkaF5LD4GsaYasX0qJShYRWXjCvlf.y}}
}

// --------------------
// interactor
// --------------------

type Interactor struct {
	repo Repository
}

func NewInteractor(repo Repository) *Interactor {
	return &Interactor{repo}
}

func (it *Interactor) Signup(ctx context.Context, email, passwordRaw string) (Session, error) {
	u := NewUser(email, passwordRaw)
	s := NewSession(u)

	txFn := func(ctx context.Context) error {
		var err error
		s, err = it.repo.CreateUser(ctx, s)
		if err != nil {
			return err
		}

		s, err = it.repo.CreateSession(ctx, s)
		return err // if returned non-nil error, rollback
	}

	err, rbErr := it.repo.RunInTx(ctx, txFn)
	if rbErr != nil {
		// handling ...
	}
	if err != nil {
		// handling ...
	}

	return s, err
}

// --------------------
// data access
// --------------------

type Repository interface {
	RunInTx(context.Context, func(context.Context) error) (err, rbErr error)
	CreateUser(context.Context, Session) (Session, error)
	CreateSession(context.Context, Session) (Session, error)
}

type RepositoryImpl struct {
	db *sqlxx.DB
}

func NewRepositoryImpl(db *sqlxx.DB) Repository {
	return &RepositoryImpl{db}
}

func (r *RepositoryImpl) RunInTx(ctx context.Context, txFn func(context.Context) error) (err, rbErr error) {
	return r.db.RunInTx(ctx, txFn)
}

func (r *RepositoryImpl) CreateUser(ctx context.Context, s Session) (Session, error) {
	q := `INSERT INTO user (email, password) VALUES (:email, :password);`
	res, err := r.db.NamedExec(ctx, q, s.User)
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

func (r *RepositoryImpl) CreateSession(ctx context.Context, s Session) (Session, error) {
	q := `INSERT INTO session (id, expire_at, user_id) VALUES (:id, :expire_at, :user.id);`
	_, err := r.db.NamedExec(ctx, q, s)
	return s, err
}

// --------------------
// meta data
// --------------------

type Session struct {
	ID       string `db:"id"`
	ExpireAt int64  `db:"expire_at"`
	User     User   `db:"user"`
}

type User struct {
	ID       int64  `db:"id"`
	Email    string `db:"email"`
	Password string `db:"password"`
}

func NewSession(u User) Session {
	return Session{
		ID:       uuid.New().String(),
		ExpireAt: time.Now().AddDate(0, 1, 0).Unix(),
		User:     u,
	}
}

func NewUser(email, passwordRaw string) User {
	b, _ := bcrypt.GenerateFromPassword([]byte(passwordRaw), bcrypt.DefaultCost)
	return User{
		Email:    email,
		Password: string(b),
	}
}
```

## Nested Transaction

`*sqlxx.DB.RunInTx` がネストされて呼び出された場合は最もトップレベルの `RunInTx` においてトランザクションが管理されます。下位の `RunInTx` では commit や rollback は行われません。

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/rema424/sqlxx"
)

const userSchema = `
create table if not exists user (
  id bigint not null auto_increment,
  email varchar(255) character set latin1 collate latin1_bin not null default '',
  password varchar(255) not null default '',
  primary key (id),
  unique key (email)
);
`

type User struct {
	ID    int64  `db:"id"`
	Email string `db:"email"`
}

func NewUser(email string) User {
	return User{
		Email: email,
	}
}

func createUserWithTx(ctx context.Context, db *sqlxx.DB, u User) (User, error) {
	fn := func(ctx context.Context) error {
		res, err := db.NamedExec(ctx, `INSERT INTO user (email) VALUES (:email);`, u)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		u.ID = id
		return nil
	}

	err, rbErr := db.RunInTx(ctx, fn)
	if rbErr != nil {
		log.Println(rbErr)
	}
	return u, err
}

func main() {
	dbx, err := sqlx.Connect("mysql", "sqlxxtester:Passw0rd!@tcp(127.0.0.1:3306)/sqlxxtest?collation=utf8mb4_bin&interpolateParams=true&parseTime=true&maxAllowedPacket=0")
	if err != nil {
		log.Fatalln(err)
	}
	defer dbx.Close()

	// create tables
	dbx.MustExec(userSchema)

	// init *sqlxx.DB
	db := sqlxx.New(dbx, sqlxx.NewLogger(os.Stdout), nil)

	var (
		u1 = NewUser("alice-nest@example.com")
		u2 = NewUser("bob-nest@example.com")
		u3 = NewUser("carol-nest@example.com")
	)

	ctx := context.Background()

	// --------------------
	// no nest
	// --------------------
	// Act
	_, _ = createUserWithTx(ctx, db, u1) // success, and committed

	// Check
	got, err := getUser(ctx, db, u1.Email)
	fmt.Printf("User: %+v, err: %+v\n", got, err)
	// => User: {ID:30 Email:alice-nest@example.com}, err: <nil>

	// --------------------
	// nest (commit)
	// --------------------
	// Act
	_, _ = db.RunInTx(ctx, func(ctx context.Context) error {
		_, _ = createUserWithTx(ctx, db, u2) // success, but not yet committed
		return nil                           // nil returned, commit
	})

	// Check
	got, err = getUser(ctx, db, u2.Email)
	fmt.Printf("User: %+v, err: %+v\n", got, err)
	// => User: {ID:31 Email:bob-nest@example.com}, err: <nil>

	// --------------------
	// nest (rollback)
	// --------------------
	// Act
	_, _ = db.RunInTx(ctx, func(ctx context.Context) error {
		_, _ = createUserWithTx(ctx, db, u3)   // success, but not yet committed
		return errors.New("some error ocurrs") // non-nil error returned, rollback
	})

	// Check
	got, err = getUser(ctx, db, u3.Email)
	fmt.Printf("User: %+v, err: %+v\n", got, err)
	// => User: {ID:0 Email:}, err: sql: no rows in result set
}

func getUser(ctx context.Context, db *sqlxx.DB, email string) (User, error) {
	var u User
	err := db.Get(ctx, &u, "SELECT id, email FROM user WHERE email = ?;", email)
	return u, err
}
```
