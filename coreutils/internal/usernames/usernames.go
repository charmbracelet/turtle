// Package usernames resolves the user and group identifiers of a file to
// the names they stand for, as ls, stat and find print them.
package usernames

import (
	"os/user"
	"strconv"
)

// Names resolves user and group ids to names, remembering each lookup.
//
// Unknown is the name an identifier which names nothing takes: when empty,
// its decimal form, as ls and find do; stat uses "UNKNOWN".
//
// EmptyNameMissing makes a lookup which succeeds with an empty name count
// as naming nothing, as ls does; otherwise the empty name is used as it is.
//
// LookupUser and LookupGroup, when not nil, replace the lookups of the
// os/user package, so tests can decide the names.
type Names struct {
	Unknown          string
	EmptyNameMissing bool
	LookupUser       func(id string) (string, error)
	LookupGroup      func(id string) (string, error)

	users  map[uint64]string
	groups map[uint64]string
}

// User returns the name the user identifier stands for.
func (n *Names) User(id uint64) string {
	if name, ok := n.users[id]; ok {
		return name
	}
	if n.users == nil {
		n.users = map[uint64]string{}
	}
	name := n.resolve(id, n.LookupUser, lookupUser)
	n.users[id] = name
	return name
}

// Group returns the name the group identifier stands for.
func (n *Names) Group(id uint64) string {
	if name, ok := n.groups[id]; ok {
		return name
	}
	if n.groups == nil {
		n.groups = map[uint64]string{}
	}
	name := n.resolve(id, n.LookupGroup, lookupGroup)
	n.groups[id] = name
	return name
}

// resolve returns the name an identifier stands for, as the lookup produced
// it, or the Unknown one when the lookup fails or, with EmptyNameMissing,
// names none.
func (n *Names) resolve(id uint64, custom, system func(string) (string, error)) string {
	decimal := strconv.FormatUint(id, 10)
	lookup := custom
	if lookup == nil {
		lookup = system
	}
	name, err := lookup(decimal)
	if err == nil && !(n.EmptyNameMissing && name == "") {
		return name
	}
	if n.Unknown != "" {
		return n.Unknown
	}
	return decimal
}

// lookupUser returns the name of the user an identifier stands for on the
// system.
func lookupUser(id string) (string, error) {
	u, err := user.LookupId(id)
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

// lookupGroup returns the name of the group an identifier stands for on the
// system.
func lookupGroup(id string) (string, error) {
	g, err := user.LookupGroupId(id)
	if err != nil {
		return "", err
	}
	return g.Name, nil
}
