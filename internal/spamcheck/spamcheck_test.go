package spamcheck

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
)

const reply = "SPAMD/1.1 0 EX_OK\r\nContent-length: 400\r\nSpam: True ; 6.2 / 5.0\r\n\r\nSpam detection software has identified this message.\n\nContent analysis details:   (6.2 points, 5.0 required)\n\n pts rule name              description\n---- ---------------------- --------------------------------------------------\n 3.1 FREE_MONEY             Lots of money if you click\n                            right now\n 3.1 URIBL_BLACK            Contains an URL listed in the URIBL blacklist\n"

func TestParse(t *testing.T) {
	r, err := parse(reply)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Spam || r.Score != 6.2 || len(r.Rules) != 2 || r.Rules[0].Description != "Lots of money if you click right now" {
		t.Fatalf("%+v", r)
	}
}

func TestCheckerAgainstFakeSpamd(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	got := make(chan string, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		b, _ := io.ReadAll(c)
		got <- string(b)
		c.Write([]byte(reply))
		c.Close()
	}()
	res := Checker{Addr: l.Addr().String(), Threshold: 5}.Check(context.Background(), Email{FromAddress: "a@b.test", Subject: "FREE MONEY NOW!!", HTML: `<a href="https://bit.ly/x">x</a>`})
	req := <-got
	if !strings.HasPrefix(req, "REPORT SPAMC/1.5") || !strings.Contains(req, "Subject: FREE MONEY NOW!!") {
		t.Fatalf("request %q", req[:80])
	}
	if !res.Available || !res.Spam || len(res.Warnings) != 3 {
		t.Fatalf("%+v", res)
	}
	off := Checker{Addr: "", Threshold: 5}.Check(context.Background(), Email{Subject: "Hello", HTML: "<p>hi</p>"})
	if off.Available || off.Spam || len(off.Warnings) != 0 {
		t.Fatalf("without spamd %+v", off)
	}
}
