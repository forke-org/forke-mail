// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package contacts

import "strings"

var rolePrefixes = map[string]bool{
	"abuse": true, "admin": true, "administrator": true, "billing": true, "careers": true, "contact": true, "enquiries": true,
	"hello": true, "help": true, "hostmaster": true, "hr": true, "info": true, "jobs": true, "mailer-daemon": true, "marketing": true,
	"no-reply": true, "noreply": true, "office": true, "postmaster": true, "root": true, "sales": true, "security": true,
	"support": true, "team": true, "webmaster": true,
}

var disposableDomains = map[string]bool{
	"10minutemail.com": true, "20minutemail.com": true, "33mail.com": true, "anonaddy.me": true, "burnermail.io": true,
	"discard.email": true, "dispostable.com": true, "dropmail.me": true, "emailondeck.com": true, "fakeinbox.com": true,
	"fakemail.net": true, "getairmail.com": true, "getnada.com": true, "guerrillamail.biz": true, "guerrillamail.com": true,
	"guerrillamail.de": true, "guerrillamail.info": true, "guerrillamail.net": true, "guerrillamail.org": true,
	"guerrillamailblock.com": true, "harakirimail.com": true, "inboxkitten.com": true, "jetable.org": true, "mail.tm": true,
	"mailcatch.com": true, "maildrop.cc": true, "mailinator.com": true, "mailinator.net": true, "mailnesia.com": true,
	"mailpoof.com": true, "mintemail.com": true, "mohmal.com": true, "moakt.com": true, "mytemp.email": true, "nada.email": true,
	"sharklasers.com": true, "spam4.me": true, "spamgourmet.com": true, "temp-mail.io": true, "temp-mail.org": true,
	"tempail.com": true, "tempinbox.com": true, "tempmail.com": true, "tempmail.net": true, "tempmailo.com": true,
	"tempr.email": true, "throwawaymail.com": true, "trashmail.com": true, "trashmail.de": true, "trashmail.net": true,
	"yopmail.com": true, "yopmail.fr": true, "yopmail.net": true,
}

var typoDomains = map[string]string{
	"gmial.com": "gmail.com", "gmai.com": "gmail.com", "gmal.com": "gmail.com", "gamil.com": "gmail.com", "gnail.com": "gmail.com",
	"gmaill.com": "gmail.com", "gmail.co": "gmail.com", "gmail.con": "gmail.com", "gmail.cm": "gmail.com", "gmail.om": "gmail.com",
	"gmali.com": "gmail.com", "gmil.com": "gmail.com", "googlemail.co": "googlemail.com",
	"hotmial.com": "hotmail.com", "hotmal.com": "hotmail.com", "hotmai.com": "hotmail.com", "hotmail.co": "hotmail.com",
	"hotmail.con": "hotmail.com", "homail.com": "hotmail.com",
	"yahooo.com": "yahoo.com", "yaho.com": "yahoo.com", "yahoo.co": "yahoo.com", "yahoo.con": "yahoo.com", "yhoo.com": "yahoo.com",
	"outlok.com": "outlook.com", "outllok.com": "outlook.com", "outlook.co": "outlook.com", "outlook.con": "outlook.com",
	"iclod.com": "icloud.com", "icloud.co": "icloud.com", "icoud.com": "icloud.com", "icloud.con": "icloud.com",
}

type Quality struct {
	Reason     string `json:"reason"`
	Suggestion string `json:"suggestion,omitempty"`
}

func Assess(email string) Quality {
	local, domain, ok := strings.Cut(email, "@")
	if !ok {
		return Quality{}
	}
	if fixed, typo := typoDomains[domain]; typo {
		return Quality{Reason: "typo", Suggestion: local + "@" + fixed}
	}
	if disposableDomains[domain] {
		return Quality{Reason: "disposable"}
	}
	if rolePrefixes[strings.TrimRight(local, "0123456789")] {
		return Quality{Reason: "role"}
	}
	return Quality{}
}

type Flagged struct {
	Email string `json:"email"`
	Quality
}

type Report struct {
	Rows       int       `json:"rows"`
	Invalid    int       `json:"invalid"`
	Duplicates int       `json:"duplicates"`
	Role       int       `json:"role"`
	Disposable int       `json:"disposable"`
	Typos      int       `json:"typos"`
	Samples    []Flagged `json:"samples"`
}

func (p Parsed) Check() Report {
	rep := Report{Rows: len(p.Rows), Invalid: p.Invalid, Duplicates: p.Duplicates, Samples: []Flagged{}}
	for _, r := range p.Rows {
		q := Assess(r.Email)
		switch q.Reason {
		case "":
			continue
		case "role":
			rep.Role++
		case "disposable":
			rep.Disposable++
		case "typo":
			rep.Typos++
		}
		if len(rep.Samples) < 25 {
			rep.Samples = append(rep.Samples, Flagged{Email: r.Email, Quality: q})
		}
	}
	return rep
}

func (p Parsed) Clean(keepRisky bool) (Parsed, int) {
	out := Parsed{Invalid: p.Invalid, Duplicates: p.Duplicates, Rows: make([]Row, 0, len(p.Rows))}
	dropped := 0
	for _, r := range p.Rows {
		q := Assess(r.Email)
		if q.Reason == "typo" || (q.Reason != "" && !keepRisky) {
			dropped++
			continue
		}
		out.Rows = append(out.Rows, r)
	}
	return out, dropped
}
