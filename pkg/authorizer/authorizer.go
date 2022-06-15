package authorizer

import "fmt"

const (
	ActionStart  = "start"
	ActionGet    = "get"
	ActionStop   = "stop"
	ActionStream = "stream"
)

type Role struct {
	Name    string
	Actions []string
}

type User struct {
	Subject string
	Roles   []Role
}

type Authorizer struct {
	Users map[string]User
}

func NewAuthorizer() *Authorizer {
	adminRole := Role{
		Name:    "admin",
		Actions: []string{ActionGet, ActionStart, ActionStop, ActionStream},
	}

	viewerRole := Role{
		Name:    "viewer",
		Actions: []string{ActionGet, ActionStream},
	}

	alice := User{
		Subject: "alice",
		Roles:   []Role{adminRole},
	}

	victor := User{
		Subject: "victor",
		Roles:   []Role{viewerRole},
	}

	fixtures := map[string]User{
		alice.Subject:  alice,
		victor.Subject: victor,
	}
	return &Authorizer{Users: fixtures}
}

// HasAccess determines if the subject has access to perform action.
func (a *Authorizer) HasAccess(subject string, action string) (bool, error) {
	user, ok := a.Users[subject]
	if !ok {
		return false, fmt.Errorf("subject %s not found", subject)
	}
	// the double for loop heres can be optimized using maps
	for _, role := range user.Roles {
		for _, allowedActions := range role.Actions {
			if action == allowedActions {
				return true, nil
			}
		}
	}
	return false, nil
}

// authorizer should have methods to create users, roles, etc but we omit them here.
// preload authorizer with some fixture data that matches the subject in the fixture certs
