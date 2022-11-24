package can

import (
	"testing"
)

// Some mock structs for use in testing

// user is a simplistic user with a role
type user struct {
	id   int64
	role int64
}

const (
	Reader     = 3
	Editor     = 10
	Admin      = 100
	SuperAdmin = 200
)

// Role returns the user role.
func (u *user) RoleID() int64 {
	return u.role
}

// UserID returns the user id for owned by check.
func (u *user) UserID() int64 {
	return u.id
}

const (
	PagesTable = "pages"
	PageOwner  = 999
)

type page struct {
	table string
	owner int64
}

// OwnedBy returns true if this user id owns this object.
func (p *page) OwnedBy(id int64) bool {
	return id == p.owner
}

// ResourceID returns a key unique to this model (we use table)
func (p *page) ResourceID() string {
	return p.table
}

// Test the can fuction operates as we expect for various permissions
func TestCanDo(t *testing.T) {
	var err error

	// Set up some users
	superadmin := &user{id: 10, role: SuperAdmin}
	admin := &user{id: 1, role: Admin}
	owner := &user{id: 2, role: Editor}
	reader := &user{id: 3, role: Reader}

	// Set up a resource owned by user 2
	p := &page{owner: 2, table: PagesTable}
	p2 := &page{owner: 1, table: PagesTable}

	// Set up some abilities to test against

	// Superadmins can do anything
	Authorise(SuperAdmin, ManageResource, Anything)

	// Admin can manage all pages
	Authorise(Admin, ManageResource, p.table)

	// Editor can manage their own pages and show others
	AuthoriseOwner(Editor, ManageResource, p.table)
	Authorise(Editor, ShowResource, p.table)

	// Reader can only show pages
	Authorise(Reader, ShowResource, p.table)

	// Superadmins can access all with just one rule
	err = Manage(p, superadmin)
	if err != nil {
		t.Fatalf("can: failed to manage page as superadmin %s", err)
	}
	err = Manage(p2, superadmin)
	if err != nil {
		t.Fatalf("can: failed to manage page as superadmin %s", err)
	}

	// Admins can access all
	err = Do(ManageResource, p, admin)
	if err != nil {
		t.Fatalf("can: failed to manage page as admin %s", err)
	}

	err = Do(CreateResource, p, admin)
	if err != nil {
		t.Fatalf("can: failed to create page as admin %s", err)
	}

	// Admins allowed all actions on all page resources
	err = Create(p, admin)
	if err != nil {
		t.Fatalf("can: failed to create page as admin %s", err)
	}

	err = Manage(p2, admin)
	if err != nil {
		t.Fatalf("can: failed to manage page2 as admin %s", err)
	}

	// Editor can access their own resource, but not another
	err = Do(ManageResource, p, owner)
	if err != nil {
		t.Fatalf("can: failed to manage page as editor %s", err)
	}

	// Editor can manage their resource
	err = Manage(p, owner)
	if err != nil {
		t.Fatalf("can: failed to manage page as editor %s", err)
	}

	// Editor can create pages as they have ManagesOwn
	err = Create(p, owner)
	if err != nil {
		t.Fatalf("can: failed to create page as editor %s", err)
	}

	// Editor cannot access admin's page so we should get error
	err = Do(ManageResource, p2, owner)
	if err == nil {
		t.Fatalf("can: failed to block manage page 2 as editor")
	}

	// Others can't access either page except on show so we should get error
	err = Manage(p, reader)
	if err == nil {
		t.Fatalf("can: failed to block page")
	}

	err = Destroy(p, reader)
	if err == nil {
		t.Fatalf("can: failed block destroy page")
	}

	err = Manage(p2, reader)
	if err == nil {
		t.Fatalf("can: failed block manage page")
	}

	err = Create(p2, reader)
	if err == nil {
		t.Fatalf("can: failed block create page")
	}

	err = Update(p2, reader)
	if err == nil {
		t.Fatalf("can: failed block update page")
	}

	err = List(p2, reader)
	if err == nil {
		t.Fatalf("can: failed block list page")
	}

	err = Destroy(p2, reader)
	if err == nil {
		t.Fatalf("can: failed block destroy page")
	}

	// Test list with nil param
	err = List(p, nil)
	if err == nil {
		t.Fatalf("can: failed to block page")
	}

	// Reader can show both pages but do nothing else
	err = Show(p, reader)
	if err != nil {
		t.Fatalf("can: failed block show page, %s", err)
	}

	err = Show(p2, reader)
	if err != nil {
		t.Fatalf("can: failed block show page, %s", err)
	}
}
