package compatapi

import (
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
)

type addressList []string

func (a *addressList) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = addressList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return errors.New("must be a string or an array of strings")
	}
	*a = many
	return nil
}

func (a addressList) single() (string, bool) {
	if len(a) != 1 {
		return "", false
	}
	addr, err := mail.ParseAddress(a[0])
	if err != nil {
		return "", false
	}
	return strings.ToLower(addr.Address), true
}

func parseFrom(raw string) (name, address string, ok bool) {
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", "", false
	}
	return addr.Name, strings.ToLower(addr.Address), true
}
