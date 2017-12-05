// +build !windows

package idtools

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/opencontainers/runc/libcontainer/user"
)

var (
	entOnce   sync.Once
	getentCmd string
)

func mkdirAs(path string, mode os.FileMode, ownerUID, ownerGID int, mkAll, chownExisting bool) error {
	// to preserve simple vendoring for this example code, gutting this function
	// to limit a group of `pkg/*` content required
	return nil
}

func accessible(isOwner, isGroup bool, perms os.FileMode) bool {
	if isOwner && (perms&0100 == 0100) {
		return true
	}
	if isGroup && (perms&0010 == 0010) {
		return true
	}
	if perms&0001 == 0001 {
		return true
	}
	return false
}

// LookupUser uses traditional local system files lookup (from libcontainer/user) on a username,
// followed by a call to `getent` for supporting host configured non-files passwd and group dbs
func LookupUser(username string) (user.User, error) {
	// first try a local system files lookup using existing capabilities
	usr, err := user.LookupUser(username)
	if err == nil {
		return usr, nil
	}
	// local files lookup failed; attempt to call `getent` to query configured passwd dbs
	usr, err = getentUser(fmt.Sprintf("%s %s", "passwd", username))
	if err != nil {
		return user.User{}, err
	}
	return usr, nil
}

// LookupUID uses traditional local system files lookup (from libcontainer/user) on a uid,
// followed by a call to `getent` for supporting host configured non-files passwd and group dbs
func LookupUID(uid int) (user.User, error) {
	// first try a local system files lookup using existing capabilities
	usr, err := user.LookupUid(uid)
	if err == nil {
		return usr, nil
	}
	// local files lookup failed; attempt to call `getent` to query configured passwd dbs
	return getentUser(fmt.Sprintf("%s %d", "passwd", uid))
}

func getentUser(args string) (user.User, error) {
	reader, err := callGetent(args)
	if err != nil {
		return user.User{}, err
	}
	users, err := user.ParsePasswd(reader)
	if err != nil {
		return user.User{}, err
	}
	if len(users) == 0 {
		return user.User{}, fmt.Errorf("getent failed to find passwd entry for %q", strings.Split(args, " ")[1])
	}
	return users[0], nil
}

// LookupGroup uses traditional local system files lookup (from libcontainer/user) on a group name,
// followed by a call to `getent` for supporting host configured non-files passwd and group dbs
func LookupGroup(groupname string) (user.Group, error) {
	// first try a local system files lookup using existing capabilities
	group, err := user.LookupGroup(groupname)
	if err == nil {
		return group, nil
	}
	// local files lookup failed; attempt to call `getent` to query configured group dbs
	return getentGroup(fmt.Sprintf("%s %s", "group", groupname))
}

// LookupGID uses traditional local system files lookup (from libcontainer/user) on a group ID,
// followed by a call to `getent` for supporting host configured non-files passwd and group dbs
func LookupGID(gid int) (user.Group, error) {
	// first try a local system files lookup using existing capabilities
	group, err := user.LookupGid(gid)
	if err == nil {
		return group, nil
	}
	// local files lookup failed; attempt to call `getent` to query configured group dbs
	return getentGroup(fmt.Sprintf("%s %d", "group", gid))
}

func getentGroup(args string) (user.Group, error) {
	reader, err := callGetent(args)
	if err != nil {
		return user.Group{}, err
	}
	groups, err := user.ParseGroup(reader)
	if err != nil {
		return user.Group{}, err
	}
	if len(groups) == 0 {
		return user.Group{}, fmt.Errorf("getent failed to find groups entry for %q", strings.Split(args, " ")[1])
	}
	return groups[0], nil
}

func callGetent(args string) (io.Reader, error) {
	entOnce.Do(func() { getentCmd, _ = resolveBinary("getent") })
	// if no `getent` command on host, can't do anything else
	if getentCmd == "" {
		return nil, fmt.Errorf("")
	}
	out, err := execCmd(getentCmd, args)
	if err != nil {
		return nil, fmt.Errorf("getent failed")
	}
	return bytes.NewReader(out), nil
}
