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
	_, _ = createUserWithTx(ctx, db, u1) // success, and commit

	// Check
	got, err := getUser(ctx, db, u1.Email)
	fmt.Printf("User: %+v, err: %+v\n", got, err)
	// => User: {ID:30 Email:alice-nest@example.com}, err: <nil>

	// --------------------
	// nest (commit)
	// --------------------
	// Act
	_, _ = db.RunInTx(ctx, func(ctx context.Context) error {
		_, _ = createUserWithTx(ctx, db, u2) // success, but not commit
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
		_, _ = createUserWithTx(ctx, db, u3)   // success, but not commit
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
