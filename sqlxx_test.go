package sqlxx

import (
	"log"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

var (
	dbx *sqlx.DB
	db  *DB
)

func TestMain(m *testing.M) {
	/*
	  mysql.server start
	  mysql -uroot -e 'create database if not exists sqlxxtest;'
	  mysql -uroot -e 'create user if not exists sqlxxtester@localhost identified by "Passw0rd!";'
	  mysql -uroot -e 'grant all privileges on sqlxxtest.* to sqlxxtester@localhost;'
	  mysql -uroot -e 'show databases;'
	  mysql -uroot -e 'select host, user from mysql.user;'
	  mysql -uroot -e 'show grants for sqlxxtester@localhost;'
	*/

	var err error
	dbx, err = sqlx.Connect("mysql", "sqlxxtester:Passw0rd!@tcp(127.0.0.1:3306)/sqlxxtest?collation=utf8mb4_bin&interpolateParams=true&parseTime=true&maxAllowedPacket=0")
	if err != nil {
		log.Fatalf("sqlx.Connect: %v", err)
	}

	db = &DB{dbx}

	os.Exit(m.Run())
}

func TestConnect(t *testing.T) {
	db := New(dbx)
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to Connect: %v", err)
	}
}

func TestBuild(t *testing.T) {}

func TestNewTxCtx(t *testing.T) {}

func TestGet(t *testing.T) {}

func TestSelect(t *testing.T) {}

func TestExec(t *testing.T) {}

func TestNamedExec(t *testing.T) {}

func TestQuery(t *testing.T) {}

func TestRunInTx(t *testing.T) {}
