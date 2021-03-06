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
	// craate database, user
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
	GetUserByID(ctx context.Context, id int64) (User, error)
	GetSessionByID(ctx context.Context, id string) (Session, error)
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

func (r *RepositoryImpl) GetUserByID(ctx context.Context, id int64) (User, error) {
	q := `SELECT id, email, password FROM user WHERE id = ?;`
	var u User
	err := r.db.Get(ctx, &u, q, id)
	return u, err
}

func (r *RepositoryImpl) CreateSession(ctx context.Context, s Session) (Session, error) {
	q := `INSERT INTO session (id, expire_at, user_id) VALUES (:id, :expire_at, :user.id);`
	_, err := r.db.NamedExec(ctx, q, s)
	return s, err
}

func (r *RepositoryImpl) GetSessionByID(ctx context.Context, id string) (Session, error) {
	q := `
  SELECT
    s.id,
    s.expire_at,
    u.id AS 'user.id',
    u.email AS 'user.email',
    u.password AS 'user.password'
  FROM session AS s
  INNER JOIN user AS u ON u.id = s.user_id
  WHERE s.id = ?;
  `
	var s Session
	err := r.db.Get(ctx, &s, q, id)
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
