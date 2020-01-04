# sqlxx

[![Go Report Card](https://goreportcard.com/badge/github.com/rema424/sqlxx)](https://goreportcard.com/report/github.com/rema424/sqlxx)
[![Coverage Status](https://coveralls.io/repos/github/rema424/sqlxx/badge.svg?branch=master)](https://coveralls.io/github/rema424/sqlxx?branch=master)
[![Build Status](https://travis-ci.org/rema424/sqlxx.svg?branch=master)](https://travis-ci.org/rema424/sqlxx)
[![GoDoc](https://godoc.org/github.com/rema424/sqlxx?status.svg)](https://godoc.org/github.com/rema424/sqlxx)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

jmoiron/sqlx を拡張してトランザクション管理をアプリケーションコアから切り離せるようにしたライブラリ

## usage

```go
package main

import (
	"context"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/k0kubun/pp"
	"github.com/rema424/sqlxx"
)

var schema1 = `
create table if not exists person (
  id bigint auto_increment,
  name varchar(255),
  primary key (id)
);`

var schema2 = `
create table if not exists favorite_food (
  id bigint auto_increment,
  person_id bigint,
  name varchar(255),
  primary key (id),
  unique (person_id, name),
  foreign key (person_id) references person (id) on update cascade on delete set null
);`

// Person .
type Person struct {
	ID    int64  `db:"qwerty"`
	Name  string `db:"asdfgh"`
	Foods []Food
}

// Food .
type Food struct {
	ID       int64  `db:"zxcvbn"`
	PersonID int64  `db:"uiopjkl"`
	Name     string `db:"yhnujm"`
}

func main() {
	db, err := sqlx.Connect("mysql", "devuser:Passw0rd!@tcp(127.0.0.1:3306)/myproject?collation=utf8mb4_bin&interpolateParams=true&parseTime=true&maxAllowedPacket=0")
	// db, err := sqlx.Connect("mysql", os.Getenv("DSN"))
	if err != nil {
		log.Fatalln(err)
	}

	db.MustExec(schema1)
	db.MustExec(schema2)

	accssr, err := sqlxx.Open(db)
	if err != nil {
		log.Fatalln(err)
	}

	ctx := context.Background()
	f1 := Food{Name: "apple"}
	f2 := Food{Name: "banana"} // replace banana to apple, foreign key constraint error happen and rollback
	p := Person{Name: "Alice", Foods: []Food{f1, f2}}

	var txFn sqlxx.TxFunc = func(ctx context.Context) (interface{}, error) {
		q1 := `insert into person (name) values (:asdfgh);`
		res, err := accssr.NamedExec(ctx, q1, p)
		if err != nil {
			return nil, err // if err returned, rollback
		}

		pid, err := res.LastInsertId()
		if err != nil {
			return nil, err // if err returned, rollback
		}
		p.ID = pid

		q2 := `insert into favorite_food (person_id, name) values (:uiopjkl, :yhnujm);`
		for i := range p.Foods {
			p.Foods[i].PersonID = pid
			res, err := accssr.NamedExec(ctx, q2, p.Foods[i])
			if err != nil {
				return nil, err // if err returned, rollback
			}
			fid, err := res.LastInsertId()
			if err != nil {
				return nil, err // if err returned, rollback
			}
			p.Foods[i].ID = fid
		}

		return p, nil // if err not returned, commit
	}

	v, err, rlbkErr := accssr.RunInTx(ctx, txFn)
	if rlbkErr != nil {
		// alert (email, slack, etc.)
		log.Println("rollback error occurred - err:", rlbkErr.Error())
	}

	if err != nil {
		log.Println("sql error occurred - err:", err.Error())
	} else {
		pp.Println(v.(Person))
		// main.Person{
		//   ID:    1,
		//   Name:  "Alice",
		//   Foods: []main.Food{
		//     main.Food{
		//       ID:       1,
		//       PersonID: 1,
		//       Name:     "apple",
		//     },
		//     main.Food{
		//       ID:       2,
		//       PersonID: 1,
		//       Name:     "banana",
		//     },
		//   },
		// }
	}
}
```
