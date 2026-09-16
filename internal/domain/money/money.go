package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalidAmount   = errors.New("invalid money amount")
	ErrInvalidCurrency = errors.New("invalid currency")
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrOverflow = errors.New("money overflow")
)

type Money struct {
	Amount   int64
	Currency string
}

func New(amount int64, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return Money{}, ErrInvalidCurrency
	}
	return Money{Amount: amount, Currency: currency}, nil
}

func Parse(amount, currency string) (Money, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return Money{}, ErrInvalidCurrency
	}

	s := strings.TrimSpace(amount)
	if s == "" || strings.ContainsAny(s, "eE") {
		return Money{}, ErrInvalidAmount
	}

	negative := false
	if s[0] == '-' {
		negative = true
		s = s[1:]
	}
	if s == "" {
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

	value := wholeValue*100 + fractionValue
	if negative {
		value = -value
	}
	return Money{Amount: value, Currency: currency}, nil
}

func (m Money) String() string {
	sign := ""
	value := m.Amount
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (other.Amount > 0 && m.Amount > math.MaxInt64-other.Amount) ||
		(other.Amount < 0 && m.Amount < math.MinInt64-other.Amount) {
		return Money{}, ErrOverflow
	}
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	if other.Amount == math.MinInt64 {
		return Money{}, ErrOverflow
	}
	return m.Add(Money{Amount: -other.Amount, Currency: other.Currency})
}
