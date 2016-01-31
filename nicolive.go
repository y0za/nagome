package nicolive

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

var ()

// NewNicoClient makes new http.Client with usersession
func NewNicoClient(a *Account) (*http.Client, error) {
	nicoURL, err := url.Parse("http://nicovideo.jp")
	if err != nil {
		return nil, err
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := http.Client{Jar: jar}
	c.Jar.SetCookies(nicoURL, []*http.Cookie{
		&http.Cookie{
			Domain: nicoURL.Host,
			Path:   "/",
			Name:   "user_session",
			Value:  a.Usersession,
			Secure: false,
		},
	})
	return &c, nil
}
