package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalidAmount    = errors.New("invalid money amount")
	ErrInvalidCurrency  = errors.New("invalid currency")
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrOverflow         = errors.New("money overflow")
)

type Money struct {
	amount   int64
	currency string
}

func New(amount int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !validCurrency(currency) {
		return Money{}, ErrInvalidCurrency
	}
	return Money{amount: amount, currency: currency}, nil
}

func Rehydrate(amount int64, currency string) (Money, error) {
	return Money{amount: amount, currency: currency}, nil
}

func Zero(currency string) (Money, error) {
	return New(0, currency)
}

func (m Money) MinorUnits() int64 {
	return m.amount
}

func (m Money) Currency() string {
	return m.currency
}

func Parse(amount, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !validCurrency(currency) {
		return Money{}, ErrInvalidCurrency
	}

	s := strings.TrimSpace(amount)
	if s == "" || strings.ContainsAny(s, "eE") {
		return Money{}, ErrInvalidAmount
	}

	switch s[0] {
	case '-', '+':
		return Money{}, ErrInvalidAmount
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return Money{}, ErrInvalidAmount
	}

	whole := parts[0]
	if whole == "" {
		return Money{}, ErrInvalidAmount
	}
	for _, r := range whole {
		if r < '0' || r > '9' {
			return Money{}, ErrInvalidAmount
		}
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 2 {
		return Money{}, ErrInvalidAmount
	}
	for _, r := range fraction {
		if r < '0' || r > '9' {
			return Money{}, ErrInvalidAmount
		}
	}
	switch len(fraction) {
	case 0:
		fraction = "00"
	case 1:
		fraction += "0"
	}

	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return Money{}, ErrOverflow
	}
	if wholeValue > math.MaxInt64/100 {
		return Money{}, ErrOverflow
	}

	fractionValue, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return Money{}, ErrInvalidAmount
	}

	if wholeValue == math.MaxInt64/100 && fractionValue > math.MaxInt64%100 {
		return Money{}, ErrOverflow
	}
	value := wholeValue*100 + fractionValue
	return Money{amount: value, currency: currency}, nil
}

func validCurrency(currency string) bool {
	return iso4217[currency]
}

var iso4217 = func() map[string]bool {
	codes := strings.Fields(`
	AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD
	CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR
	FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY
	KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU
	MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD
	RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SLL SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP
	TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAF XAG XAU XBA XBB XBC
	XBD XCD XDR XOF XPD XPF XPT XSU XTS XUA XXX YER ZAR ZMW ZWG`)
	result := make(map[string]bool, len(codes))
	for _, code := range codes {
		result[code] = true
	}
	return result
}()

func (m Money) String() string {
	sign := ""
	value := m.amount
	if value < 0 {
		sign = "-"
		if value == math.MinInt64 {
			return "-92233720368547758.08"
		}
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (other.amount > 0 && m.amount > math.MaxInt64-other.amount) ||
		(other.amount < 0 && m.amount < math.MinInt64-other.amount) {
		return Money{}, ErrOverflow
	}
	return Money{amount: m.amount + other.amount, currency: m.currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	if other.amount == math.MinInt64 {
		return Money{}, ErrOverflow
	}
	return m.Add(Money{amount: -other.amount, currency: other.currency})
}

func (m Money) Negate() (Money, error) {
	if m.amount == math.MinInt64 {
		return Money{}, ErrOverflow
	}
	return Money{amount: -m.amount, currency: m.currency}, nil
}

func (m Money) Compare(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, ErrCurrencyMismatch
	}
	if m.amount < other.amount {
		return -1, nil
	}
	if m.amount > other.amount {
		return 1, nil
	}
	return 0, nil
}
