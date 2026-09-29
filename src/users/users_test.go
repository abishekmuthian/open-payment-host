package users

import (
	"reflect"
	"strings"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
)

func TestUserCRUDByEmail(t *testing.T) {
	testenv.Config(t, nil)
	testenv.DB(t)

	id, err := New().Create(map[string]string{"email": "reader@merchant.test", "password_hash": "hash", "role": "20", "status": "100"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := FindFirst("email=?", "reader@merchant.test")
	if err != nil || u.ID != id || u.Role != Reader || u.PasswordHash != "hash" || u.CreatedAt.IsZero() {
		t.Fatalf("find by email: %+v %v", u, err)
	}
	if err := u.Update(map[string]string{"email": "renamed@merchant.test"}); err != nil {
		t.Fatal(err)
	}
	if u, err = Find(id); err != nil || u.Email != "renamed@merchant.test" {
		t.Fatalf("update: %+v %v", u, err)
	}
	all, err := FindAll(Query())
	if err != nil || len(all) != 1 {
		t.Fatalf("find all: %d %v", len(all), err)
	}
	if err := u.Destroy(); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(id); err == nil || !strings.Contains(err.Error(), "No results found") {
		t.Fatalf("destroyed user still found: %v", err)
	}
}

func TestRoleHelpers(t *testing.T) {
	var nilUser *User
	if !nilUser.Anon() || nilUser.RoleID() != Anon || nilUser.UserID() != 0 {
		t.Fatal("nil user helpers")
	}
	cases := []struct {
		user                *User
		anon, admin, reader bool
		display             string
	}{
		{&User{}, true, false, false, ""},
		{&User{Role: Reader}, true, false, true, "Reader"}, // no ID: still anonymous
		{MockAnon(), true, false, false, ""},
		{MockAdmin(), false, true, false, "Administrator"},
		{&User{Role: Editor}, true, false, false, "Editor"},
	}
	for i, c := range cases {
		if c.user.Anon() != c.anon || c.user.Admin() != c.admin || c.user.Reader() != c.reader || c.user.RoleDisplay() != c.display {
			t.Errorf("case %d: anon=%v admin=%v reader=%v display=%q", i, c.user.Anon(), c.user.Admin(), c.user.Reader(), c.user.RoleDisplay())
		}
	}
	admin := MockAdmin()
	if admin.RoleID() != Admin || admin.UserID() != 1 || !admin.OwnedBy(1) || admin.OwnedBy(2) {
		t.Fatal("admin helpers")
	}
	if len(admin.RoleOptions()) != 3 {
		t.Fatal("role options")
	}
}

func TestAllowedParams(t *testing.T) {
	for _, p := range AllowedParams() {
		if p == "status" || p == "role" {
			t.Fatalf("readers may set %s", p)
		}
	}
	admin := AllowedParamsAdmin()
	if !reflect.DeepEqual(admin[1:], AllowedParams()) || admin[0] != "status" {
		t.Fatalf("admin params = %v", admin)
	}
	for _, p := range admin {
		if p == "role" {
			t.Fatal("role must never be mass-assignable")
		}
	}
}
