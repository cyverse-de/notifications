package common

import "strings"

// UserSuffix is the "@domain" that turns a bare DE username into the fully qualified form stored
// in the users table. Callers send bare usernames -- apps and terrain both pass `shortUsername` --
// while the DE identifies users by the qualified form.
type UserSuffix string

// NewUserSuffix builds a UserSuffix from a domain, tolerating a leading "@" so that a
// configuration value of either "example.org" or "@example.org" behaves the same.
func NewUserSuffix(domain string) UserSuffix {
	return UserSuffix("@" + strings.TrimPrefix(strings.TrimSpace(domain), "@"))
}

// Qualify converts a username to the form the DE stores. It truncates at the first "@" before
// appending rather than appending outright, matching apps.user/append-username-suffix:
//
//	(str (string/replace username #"@.*$" "") "@" (uid-domain))
//
// The distinction only shows for a username that is already an address, and there it decides
// which account the user resolves to: "someone@elsewhere.edu" belongs to
// "someone@<uid-domain>", not to a "someone@elsewhere.edu@<uid-domain>" spelling that no other
// service would ever produce.
func (s UserSuffix) Qualify(username string) string {
	if i := strings.IndexByte(username, '@'); i >= 0 {
		username = username[:i]
	}
	return username + string(s)
}
