package auth

import "github.com/pleware/initagent/internal/offering"

// SignupOpen reports whether a claimed hosted hub should offer customer
// registration. Self-host never does: claim already made the operator the
// founder, and an open register on someone else's disk is a land grab (26).
func SignupOpen(kind offering.Kind, claimed bool) bool {
	return kind == offering.Hosted && claimed
}

// RegisterRequest is the untrusted customer-signup submission. Confirm-
// password is form-only, as on claim. The organization name is not here:
// boarding names the company, so the first org is DefaultOrgName (26).
type RegisterRequest struct {
	Email    string
	Password string
	// Locale is the UI language at signup. Empty becomes English.
	Locale string
}

// Register decides whether a customer may create an account on this hub,
// and returns the values to persist.
//
// The order of the checks is part of the decision:
//
//   - Offering first, so a self-host hub answers the same way whether or
//     not it has been claimed. The door is not there.
//   - Claimed next, so an unclaimed hosted hub cannot be taken over
//     through this route — that is still setup plus the bootstrap token.
//   - Email and password last, so a refused door does not run argon2id.
func Register(st State, req RegisterRequest) (Credentials, error) {
	if st.Offering != offering.Hosted {
		return Credentials{}, ErrNotHosted
	}
	if !st.Claimed {
		return Credentials{}, ErrNotClaimed
	}
	email, err := NormalizeEmail(req.Email)
	if err != nil {
		return Credentials{}, err
	}
	locale, err := NormalizeLocale(req.Locale)
	if err != nil {
		return Credentials{}, err
	}
	if err := CheckPassword(st.Offering, email, req.Password); err != nil {
		return Credentials{}, err
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{
		Email:        email,
		PasswordHash: hash,
		OrgName:      CheckOrgName(""),
		Locale:       locale,
	}, nil
}
