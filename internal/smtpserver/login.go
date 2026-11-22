package smtpserver

import (
	"errors"

	"github.com/emersion/go-sasl"
)

type loginServer struct {
	check    func(username, password string) error
	username string
	step     int
}

func newLoginServer(check func(username, password string) error) sasl.Server {
	return &loginServer{check: check}
}

func (l *loginServer) Next(response []byte) ([]byte, bool, error) {
	switch l.step {
	case 0:
		l.step = 1
		if response != nil {
			l.username = string(response)
			l.step = 2
			return []byte("Password:"), false, nil
		}
		return []byte("Username:"), false, nil
	case 1:
		l.username = string(response)
		l.step = 2
		return []byte("Password:"), false, nil
	case 2:
		l.step = 3
		return nil, true, l.check(l.username, string(response))
	}
	return nil, false, errors.New("unexpected client response")
}
