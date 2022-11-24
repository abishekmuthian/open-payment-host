// Package model provides a class including Id, CreatedAt and UpdatedAt, and some utility functions, optionally include in your models
package model

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Model base class
type Model struct {
	// Id is the default primary key of the model
	Id int64

	// CreatedAt stores the creation time of the model and should not be changed after cretion
	CreatedAt time.Time

	// UpdatedAt stores the last update time of the model
	UpdatedAt time.Time

	// TableName is used for database queries and urls
	TableName string

	// KeyName is used for database queries as the primary key
	KeyName string
}

// Init sets up the model fields
func (m *Model) Init() {
	m.Id = 0
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	m.TableName = ""
	m.KeyName = "id"
}

// URLCreate returns the create url for this model /table/create
func (m *Model) URLCreate() string {
	return fmt.Sprintf("/%s/create", m.TableName)
}

// URLUpdate returns the update url for this model /table/id/update
func (m *Model) URLUpdate() string {
	return fmt.Sprintf("/%s/%d/update", m.TableName, m.Id)
}

// URLDestroy returns the destroy url for this model /table/id/destroy
func (m *Model) URLDestroy() string {
	return fmt.Sprintf("/%s/%d/destroy", m.TableName, m.Id)
}

// URLShow returns the show url for this model /table/id
func (m *Model) URLShow() string {
	return fmt.Sprintf("/%s/%d", m.TableName, m.Id)
}

// URLIndex returns the index url for this model - /table
func (m *Model) URLIndex() string {
	return fmt.Sprintf("/%s", m.TableName)
}

// ToSlug converts our name to something suitable for use on the web as part of a url
func (m *Model) ToSlug(name string) string {
	// Lowercase
	slug := strings.ToLower(name)

	// Replace _ with - for consistent style
	slug = strings.Replace(slug, "_", "-", -1)
	slug = strings.Replace(slug, " ", "-", -1)

	// In case of regexp failure, replace at least /
	slug = strings.Replace(slug, "/", "-", -1)

	// Run regexp - remove all letters except a-z0-9-
	re, err := regexp.Compile("[^a-z0-9-]*")
	if err != nil {
		fmt.Println("ToSlug regexp failed")
	} else {
		slug = re.ReplaceAllString(slug, "")
	}

	return slug
}

// Table returns the table name for this object
func (m *Model) Table() string {
	return m.TableName
}

// PrimaryKey returns the id for primary key by default - used by query
func (m *Model) PrimaryKey() string {
	return m.KeyName
}

// SelectName returns our name for select menus
func (m *Model) SelectName() string {
	return fmt.Sprintf("%s-%d", m.TableName, m.Id) // Usually override with name or a summary
}

// SelectValue returns our value for select options
func (m *Model) SelectValue() string {
	return fmt.Sprintf("%d", m.Id)
}

// PrimaryKeyValue returns the unique id
func (m *Model) PrimaryKeyValue() int64 {
	return m.Id
}

// OwnedBy returns true if the user id passed in owns this model
func (m *Model) OwnedBy(uid int64) bool {
	// In models composed with base model, you may want to check a user_id field or join table
	// In this base model, we return false by default
	return false
}

// ResourceID returns a key unique to this resource (we use table).
func (m *Model) ResourceID() string {
	return m.TableName
}

// CacheKey generates a cache key for this model object, dependent on id and UpdatedAt
// should we generate a hash of this to ensure we fit in small key size?
func (m *Model) CacheKey() string {
	// This should really be some form of hash based on this data...
	return fmt.Sprintf("%s/%d/%s", m.TableName, m.Id, m.UpdatedAt)
}

// String returns a string representation of the model
func (m *Model) String() string {
	return fmt.Sprintf("%s/%d", m.TableName, m.Id)
}

// CleanParams returns a params list cleaned of keys not in allowed list
// DEPRECATED - REMOVE AND REPLACE WITH params.Clean()
func CleanParams(params map[string]string, allowed []string) map[string]string {

	for k := range params {
		if !paramAllowed(k, allowed) {
			delete(params, k)
		}
	}

	return params
}

// paramAllowed returns true if the string is in this list
func paramAllowed(p string, allowed []string) bool {
	for _, v := range allowed {
		if p == v {
			return true
		}
	}
	return false
}
