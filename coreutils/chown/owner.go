package chown

import (
	"fmt"
	"os/user"
	"strconv"
	"strings"
)

// ownership is what a chown run sets: the numeric user and group IDs, with
// -1 leaving the current value, and the names as given, for the diagnostics.
type ownership struct {
	uid int
	gid int

	// userSpec and groupSpec are the names the run was given, or the decimal
	// IDs it got, shown as the new ownership. An empty one stands for a part
	// the run doesn't set.
	userSpec  string
	groupSpec string
}

// matches reports whether the ownership a file has now, when it is known, is
// the one --from asks for. An unknown ownership only matches a run that
// doesn't constrain either part.
func (o ownership) matches(uid, gid int, known bool) bool {
	if !known {
		return o.uid < 0 && o.gid < 0
	}
	return (o.uid < 0 || o.uid == uid) && (o.gid < 0 || o.gid == gid)
}

// parseOwner parses an owner[:group] operand. Either part is a name of the
// host database or a numeric ID, with a + prefix forcing the interpretation
// as a number. "owner:" also changes the group, to the login group of the
// owner, and an empty operand or a lone colon changes nothing.
func (p *Prog) parseOwner(spec string) (ownership, error) {
	return p.parseSpec(spec, false)
}

// parseFrom parses the --from operand, where an empty part matches any
// value, as GNU asks: --from=OWNER: constrains the owner alone, whatever
// the group of the file is.
func (p *Prog) parseFrom(spec string) (ownership, error) {
	return p.parseSpec(spec, true)
}

func (p *Prog) parseSpec(spec string, from bool) (ownership, error) {
	name, group, colon := strings.Cut(spec, ":")
	out := ownership{uid: -1, gid: -1}

	var u *user.User
	if name != "" {
		var err error
		u, err = p.user(name)
		if err != nil {
			if colon && group == "" && !from {
				// "owner:" of an owner which isn't a name has no login
				// group to give, which GNU refuses.
				return ownership{}, fmt.Errorf("invalid spec: %q", spec)
			}
			id, err := numericID(name)
			if err != nil {
				return ownership{}, fmt.Errorf("invalid user: %q", spec)
			}
			out.uid, out.userSpec = id, strconv.Itoa(id)
		} else {
			out.userSpec = u.Username
			if id, err := strconv.Atoi(u.Uid); err == nil {
				out.uid = id
			}
		}
	}

	if !colon {
		return out, nil
	}
	switch {
	case group != "":
		g, err := p.group(group)
		if err != nil {
			id, err := numericID(group)
			if err != nil {
				return ownership{}, fmt.Errorf("invalid group: %q", spec)
			}
			out.gid, out.groupSpec = id, strconv.Itoa(id)
		} else {
			out.groupSpec = g.Name
			if id, err := strconv.Atoi(g.Gid); err == nil {
				out.gid = id
			}
		}
	case u != nil && !from:
		// The login group of the owner, as "owner:" asks for.
		id, err := strconv.Atoi(u.Gid)
		if err != nil {
			return out, nil
		}
		out.gid = id
		out.groupSpec = p.gidName(id)
	}
	return out, nil
}

// numericID parses a numeric user or group ID. A leading + forces the
// interpretation as a number, which is never a valid name.
func numericID(s string) (int, error) {
	s = strings.TrimPrefix(s, "+")
	if s == "" || strings.Contains(s, "-") {
		return 0, strconv.ErrSyntax
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, strconv.ErrSyntax
	}
	return n, nil
}

// uidName resolves a numeric user ID to the name the host database gives it,
// falling back to the decimal ID.
func (p *Prog) uidName(uid int) string {
	if u, err := p.userID(strconv.Itoa(uid)); err == nil {
		return u.Username
	}
	return strconv.Itoa(uid)
}

// gidName resolves a numeric group ID to the name the host database gives
// it, falling back to the decimal ID.
func (p *Prog) gidName(gid int) string {
	if g, err := p.groupID(strconv.Itoa(gid)); err == nil {
		return g.Name
	}
	return strconv.Itoa(gid)
}

// user resolves a user name, honoring lookupUser when the tests replaced it.
func (p *Prog) user(name string) (*user.User, error) {
	if p.lookupUser != nil {
		return p.lookupUser(name)
	}
	return user.Lookup(name)
}

// group resolves a group name, honoring lookupGroup when the tests replaced
// it.
func (p *Prog) group(name string) (*user.Group, error) {
	if p.lookupGroup != nil {
		return p.lookupGroup(name)
	}
	return user.LookupGroup(name)
}

// userID resolves a numeric user ID, honoring lookupUserID when the tests
// replaced it.
func (p *Prog) userID(id string) (*user.User, error) {
	if p.lookupUserID != nil {
		return p.lookupUserID(id)
	}
	return user.LookupId(id)
}

// groupID resolves a numeric group ID, honoring lookupGroupID when the tests
// replaced it.
func (p *Prog) groupID(id string) (*user.Group, error) {
	if p.lookupGroupID != nil {
		return p.lookupGroupID(id)
	}
	return user.LookupGroupId(id)
}

// describe reports how the ownership of a file changed, or was retained.
func (p *Prog) describe(shown string, oldUid, oldGid int, target ownership, verbosity int) {
	newUid, newGid := oldUid, oldGid
	if target.uid >= 0 {
		newUid = target.uid
	}
	if target.gid >= 0 {
		newGid = target.gid
	}
	unchanged := newUid == oldUid && newGid == oldGid
	if verbosity == quiet || (verbosity == changesOnly && unchanged) {
		return
	}
	if unchanged {
		switch {
		case target.uid >= 0:
			fmt.Fprintf(p.Stdout, "ownership of '%s' retained as %s\n",
				shown, p.oldSpec(target, oldUid, oldGid))
		case target.gid >= 0:
			fmt.Fprintf(p.Stdout, "group of '%s' retained as %s\n", shown, p.gidName(oldGid))
		default:
			fmt.Fprintf(p.Stdout, "ownership of '%s' retained\n", shown)
		}
		return
	}
	switch {
	case target.uid >= 0:
		fmt.Fprintf(p.Stdout, "changed ownership of '%s' from %s to %s\n",
			shown, p.oldSpec(target, oldUid, oldGid), target.newSpec())
	case target.gid >= 0:
		fmt.Fprintf(p.Stdout, "changed group of '%s' from %s to %s\n",
			shown, p.gidName(oldGid), target.groupSpec)
	}
}

// oldSpec returns the ownership a file had, as the parts the run touches.
func (p *Prog) oldSpec(target ownership, oldUid, oldGid int) string {
	spec := ""
	if target.uid >= 0 {
		spec = p.uidName(oldUid)
	}
	if target.gid >= 0 {
		spec += ":" + p.gidName(oldGid)
	}
	return spec
}

// newSpec returns the ownership a run sets, as the parts it touches.
func (t ownership) newSpec() string {
	spec := t.userSpec
	if t.gid >= 0 {
		spec += ":" + t.groupSpec
	}
	return spec
}
