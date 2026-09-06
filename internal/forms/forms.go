package forms

import (
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"
)

type Form struct {
	Values url.Values
	Errors map[string]string
}

func New(values url.Values) *Form {
	if values == nil {
		values = make(url.Values)
	}
	return &Form{Values: values, Errors: make(map[string]string)}
}

func (f *Form) Get(key string) string { return f.Values.Get(key) }

func (f *Form) Required(fields ...string) {
	for _, field := range fields {
		if strings.TrimSpace(f.Get(field)) == "" {
			f.Errors[field] = "Полето е задължително."
		}
	}
}

func (f *Form) MaxLength(field string, n int) {
	if utf8.RuneCountInString(f.Get(field)) > n {
		f.Errors[field] = "Стойността е твърде дълга."
	}
}

func (f *Form) MinLength(field string, n int) {
	if utf8.RuneCountInString(f.Get(field)) < n {
		f.Errors[field] = "Стойността е твърде кратка."
	}
}

func (f *Form) Email(field string) {
	value := strings.TrimSpace(f.Get(field))
	if value == "" {
		return
	}
	address, err := mail.ParseAddress(value)
	if err != nil || !strings.EqualFold(address.Address, value) {
		f.Errors[field] = "Въведете валиден имейл адрес."
	}
}

func (f *Form) Positive(field string, ok bool) {
	if !ok {
		f.Errors[field] = "Въведете положително число."
	}
}

func (f *Form) Valid() bool { return len(f.Errors) == 0 }
