package auth

import "golang.org/x/crypto/bcrypt"

type BcryptHasher struct {
	Cost int
}

func (h BcryptHasher) Hash(password string) ([]byte, error) {
	cost := h.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	return bcrypt.GenerateFromPassword([]byte(password), cost)
}

func (h BcryptHasher) Compare(hash []byte, password string) error {
	return bcrypt.CompareHashAndPassword(hash, []byte(password))
}
