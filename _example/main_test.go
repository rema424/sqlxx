package main

import (
	"context"
	"errors"
	"log"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/rema424/sqlxx"
	"golang.org/x/crypto/bcrypt"
)

var db *sqlxx.DB

func TestMain(m *testing.M) {
	dbx, err := sqlx.Connect("mysql", "sqlxxtester:Passw0rd!@tcp(127.0.0.1:3306)/sqlxxtest?collation=utf8mb4_bin&interpolateParams=true&parseTime=true&maxAllowedPacket=0")
	if err != nil {
		log.Fatalln(err)
	}
	defer dbx.Close()

	dbx.MustExec(userSchema)
	dbx.MustExec(sessionSchema)

	db = sqlxx.New(dbx, sqlxx.NewLogger(os.Stdout), nil)

	code := m.Run()

	dbx.MustExec("delete from session;")
	dbx.MustExec("delete from user;")

	os.Exit(code)
}

func TestNewUser(t *testing.T) {
	// Arrange
	tests := []struct {
		email    string
		password string
	}{
		{"alice@example.com", "Passw0rd!"},
		{"bob@example.com", "Passw0rd!!!"},
	}

	for _, tt := range tests {
		// Act
		got := NewUser(tt.email, tt.password)

		// Assert
		if got.ID != 0 {
			t.Errorf("want 0, got %d", got.ID)
		}
		if got.Email != tt.email {
			t.Errorf("want %s, got %s", tt.email, got.Email)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(got.Password), []byte(tt.password)); err != nil {
			t.Error(err)
		}
	}
}

func TestNewSesssion(t *testing.T) {
	// Arrange
	tests := []struct {
		user User
	}{
		{User{0, "alice@example.com", "Passw0rd!"}},
		{User{9, "bob@example.com", "Passw0rd!!"}},
	}
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	for _, tt := range tests {
		// Act
		got := NewSession(tt.user)

		// Assert
		if reflect.DeepEqual(got, tt.user) {
			t.Errorf("got %#v, want %#v", got.User, tt.user)
		}
		if !re.MatchString(got.ID) {
			t.Errorf("wrong id expression. got %s", got.ID)
		}
	}
}

func TestRepository(t *testing.T) {
	// Arrange
	r := NewRepositoryImpl(db)
	ctx := context.Background()
	u := NewUser("carol@example.com", "Passw0rd!")
	s := NewSession(u)

	// Act & Assert
	s = testCteateUser(ctx, s, r, t)
	testGetUserByID(ctx, s.User, r, t)
	s = testCteateSession(ctx, s, r, t)
	testGetSessionByID(ctx, s, r, t)
	testTx(ctx, r, t)
}

func testCteateUser(ctx context.Context, s Session, repo Repository, t *testing.T) Session {
	// Act
	got, err := repo.CreateUser(ctx, s)
	t.Logf("#%v", got)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if got.User.ID == 0 {
		t.Fatalf("want non-zero user id, got %d", s.User.ID)
	}
	s.User.ID = got.User.ID
	if !reflect.DeepEqual(s, got) {
		t.Fatalf("want %#v, got %#v", s, got)
	}

	// Duplicate
	if _, err := repo.CreateUser(ctx, s); err == nil {
		t.Fatalf("want non-nil error")
	}

	return got
}

func testGetUserByID(ctx context.Context, u User, repo Repository, t *testing.T) {
	// Act
	got, err := repo.GetUserByID(ctx, u.ID)
	t.Logf("#%v", got)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u, got) {
		t.Fatalf("want %#v, got %#v", u, got)
	}

	// NoRows
	if _, err := repo.GetUserByID(ctx, 99999); err == nil {
		t.Fatalf("want non-nil error")
	}
}

func testCteateSession(ctx context.Context, s Session, repo Repository, t *testing.T) Session {
	// Act
	got, err := repo.CreateSession(ctx, s)
	t.Logf("#%v", got)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, got) {
		t.Fatalf("want %#v, got %#v", s, got)
	}

	// Duplicate
	if _, err := repo.CreateSession(ctx, s); err == nil {
		t.Fatalf("want non-nil error")
	}

	return got
}

func testGetSessionByID(ctx context.Context, s Session, repo Repository, t *testing.T) {
	// Act
	got, err := repo.GetSessionByID(ctx, s.ID)
	t.Logf("#%v", got)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, got) {
		t.Fatalf("want %#v, got %#v", s, got)
	}

	// NoRows
	if _, err := repo.GetSessionByID(ctx, "qqwertyzxcvbn"); err == nil {
		t.Fatalf("want non-nil error")
	}
}

func testTx(ctx context.Context, repo Repository, t *testing.T) {
	// ----------
	// Commit
	// ----------

	// Arrange
	u := NewUser("tx-commit@example.com", "Passw0rd!")
	s := NewSession(u)
	txFn := func(ctx context.Context) error {
		s, _ = repo.CreateUser(ctx, s)    // success
		s, _ = repo.CreateSession(ctx, s) // success
		return nil                        // commit
	}

	// Act
	err, _ := repo.RunInTx(ctx, txFn)

	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if s.User.ID == 0 {
		t.Fatalf("want non-zero user id. got %d", s.User.ID)
	}
	got, _ := repo.GetSessionByID(ctx, s.ID)
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("want %#v, got %#v", s, got)
	}

	// ----------
	// Rollback
	// ----------

	// Arrange
	s.User = NewUser("tx-rollback@example.com", "Passw0rd!")
	txFn = func(ctx context.Context) error {
		var err error
		s, _ = repo.CreateUser(ctx, s)      // success
		s, err = repo.CreateSession(ctx, s) // duplicate error
		return err                          // roll back
	}

	// Act
	err, _ = repo.RunInTx(ctx, txFn)
	if err == nil {
		t.Fatalf("want non-nil error")
	}
	_, err = repo.GetUserByID(ctx, s.User.ID)
	if err == nil {
		t.Fatalf("want non-nil error")
	}
}

type fakeDB struct {
	Repository
	FakeRunInTx        func(context.Context, func(context.Context) error) (err, rbErr error)
	FakeCreateUser     func(context.Context, Session) (Session, error)
	FakeCreateSession  func(context.Context, Session) (Session, error)
	FakeGetUserByID    func(ctx context.Context, id int64) (User, error)
	FakeGetSessionByID func(ctx context.Context, id string) (Session, error)
}

func (fd *fakeDB) RunInTx(ctx context.Context, txFn func(context.Context) error) (err, rbErr error) {
	return fd.FakeRunInTx(ctx, txFn)
}

func (fd *fakeDB) CreateUser(ctx context.Context, s Session) (Session, error) {
	return fd.FakeCreateUser(ctx, s)
}

func (fd *fakeDB) CreateSession(ctx context.Context, s Session) (Session, error) {
	return fd.FakeCreateSession(ctx, s)
}

func (fd *fakeDB) GetUserByID(ctx context.Context, id int64) (User, error) {
	return fd.GetUserByID(ctx, id)
}

func (fd *fakeDB) GetSessionByID(ctx context.Context, id string) (Session, error) {
	return fd.FakeGetSessionByID(ctx, id)
}

func TestSignup(t *testing.T) {
	tests := []struct {
		email    string
		password string
		fakeR    Repository
		wantUID  int64
		wantE    bool
	}{
		{
			email:    "signup-success@example.com",
			password: "Passw0rd!",
			fakeR: &fakeDB{
				FakeRunInTx: func(ctx context.Context, txFn func(context.Context) error) (error, error) {
					return txFn(ctx), nil
				},
				FakeCreateUser: func(ctx context.Context, s Session) (Session, error) {
					s.User.ID = 5
					return s, nil
				},
				FakeCreateSession: func(ctx context.Context, s Session) (Session, error) {
					return s, nil
				},
			},
			wantUID: 5,
			wantE:   false,
		},
		{
			email:    "signup-failure@example.com",
			password: "Passw0rd!",
			fakeR: &fakeDB{
				FakeRunInTx: func(ctx context.Context, txFn func(context.Context) error) (error, error) {
					return txFn(ctx), errors.New("some error")
				},
				FakeCreateUser: func(ctx context.Context, s Session) (Session, error) {
					return s, errors.New("some error")
				},
			},
			wantUID: 7,
			wantE:   true,
		},
	}

	it := NewInteractor(nil)
	for _, tt := range tests {
		// Arrange
		it.repo = tt.fakeR

		// Act
		got, err := it.Signup(context.Background(), tt.email, tt.password)
		t.Logf("%#v", got)

		// Assert
		if tt.wantE && err == nil {
			t.Errorf("want non-nil error")
		} else if !tt.wantE && err != nil {
			t.Error(err)
		}

		if err == nil {
			if got.User.ID != tt.wantUID {
				t.Errorf("wrong user id. want %d, got %d", tt.wantUID, got.User.ID)
			}
			if got.User.Email != tt.email {
				t.Errorf("wrong email. want %s, got %s", tt.email, got.User.Email)
			}
			if err := bcrypt.CompareHashAndPassword([]byte(got.User.Password), []byte(tt.password)); err != nil {
				t.Error(err)
			}
		}
	}
}
