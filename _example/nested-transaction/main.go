package main

import (
	"context"
	"errors"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/rema424/sqlxx"
	"golang.org/x/crypto/bcrypt"
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
		// u1 = NewUser("alice-nest@example.com", "Passw0rd!")
		u2 = NewUser("bob-nest@example.com", "Passw0rd!")
		u3 = NewUser("carol-nest@example.com", "Passw0rd!")
	)

	ctx := context.Background()

	// no nest
	// _, _ = createUserWithTx(db, ctx, u1) // success, and commit

	// nest
	db.RunInTx(ctx, func(ctx context.Context) error {
		_, _ = createUserWithTx(db, ctx, u2)   // success, but not commit
		_, _ = createUserWithTx(db, ctx, u3)   // success, but not commit
		return errors.New("some error ocurrs") // non-nil error returned, rollback
	})
}

func createUserWithTx(db *sqlxx.DB, ctx context.Context, u User) (User, error) {
	fn := func(ctx context.Context) error {
		res, err := db.NamedExec(ctx, `INSERT INTO user (email, password) VALUES (:email, :password);`, u)
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

type User struct {
	ID       int64  `db:"id"`
	Email    string `db:"email"`
	Password string `db:"password"`
}

func NewUser(email, passwordRaw string) User {
	b, _ := bcrypt.GenerateFromPassword([]byte(passwordRaw), bcrypt.DefaultCost)
	return User{
		Email:    email,
		Password: string(b),
	}
}
