package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestDatabaseURL(t *testing.T) {
	base := Config{
		DBHost:     "localhost",
		DBPort:     "15432",
		DBName:     "cekdu_link",
		DBUser:     "cekdu",
		DBPassword: "cekdu_local_dev",
	}

	t.Run("plain values", func(t *testing.T) {
		c := base
		c.DBPassword = "secret"

		want := "postgres://cekdu:secret@localhost:15432/cekdu_link"
		if got := c.DatabaseURL(); got != want {
			t.Errorf("DatabaseURL() = %q, want %q", got, want)
		}
	})

	t.Run("special characters round-trip without corrupting the DSN", func(t *testing.T) {
		c := base
		c.DBUser = "app@team"
		c.DBPassword = "p@ss:w/rd?x=1#frag & more"
		c.DBName = "my/db-name"

		got := c.DatabaseURL()

		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("url.Parse(%q) error = %v", got, err)
		}
		if u.Scheme != "postgres" {
			t.Errorf("scheme = %q, want postgres", u.Scheme)
		}
		if u.Host != "localhost:15432" {
			t.Errorf("host = %q, want localhost:15432", u.Host)
		}
		if u.User.Username() != c.DBUser {
			t.Errorf("username = %q, want %q", u.User.Username(), c.DBUser)
		}
		pw, ok := u.User.Password()
		if !ok || pw != c.DBPassword {
			t.Errorf("password = %q (ok=%v), want %q", pw, ok, c.DBPassword)
		}
		if strings.TrimPrefix(u.Path, "/") != c.DBName {
			t.Errorf("database = %q, want %q", strings.TrimPrefix(u.Path, "/"), c.DBName)
		}
	})

	t.Run("ipv6 host is bracketed", func(t *testing.T) {
		c := base
		c.DBHost = "::1"

		got := c.DatabaseURL()
		want := "postgres://cekdu:cekdu_local_dev@[::1]:15432/cekdu_link"
		if got != want {
			t.Errorf("DatabaseURL() = %q, want %q", got, want)
		}
	})
}
