package intl

import (
	"math"
	"testing"

	fluent "github.com/projectfluent/fluent-go"
)

func TestNumberFormat(t *testing.T) {
	tests := []struct {
		locale string
		opts   NumberOptions
		value  float64
		want   string
	}{
		{"en-US", NumberOptions{}, -1234567.891, "-1,234,567.891"},
		{"ru", NumberOptions{Style: "unit", Unit: "meter", UnitDisplay: "long", Notation: "scientific"}, 120, "1,2E2 метров"},
		{"fr", NumberOptions{Style: "unit", Unit: "meter", UnitDisplay: "long", Notation: "scientific"}, 0.25, "2,5E-1\u00a0mètres"},
		{"cs", NumberOptions{Style: "currency", Currency: "DEM", CurrencyDisplay: "name", Notation: "engineering", MinimumFractionDigits: 1.0, MaximumFractionDigits: 1.0}, 5.2e228, "5,2E228 německých marek"},
		{"ceb", NumberOptions{Style: "unit", Unit: "yard", UnitDisplay: "long", Notation: "compact", MaximumSignificantDigits: 2.0}, 1.6e-20, "0.000000000000000000016 ka mga yarda"},
		{"de", NumberOptions{}, 1234.5, "1.234,5"},
		{"hi", NumberOptions{}, 1234567, "12,34,567"},
		{"es", NumberOptions{}, 1234, "1234"},
		{"ar-EG", NumberOptions{}, 12.5, "١٢٫٥"},
		{"en", NumberOptions{Style: "percent"}, 0.256, "26%"},
		{"en", NumberOptions{NumberingSystem: "thai"}, 42, "๔๒"},
		{"en", NumberOptions{Style: "currency", Currency: "EUR"}, 1234.5, "€1,234.50"},
		{"en-DE", NumberOptions{Style: "currency", Currency: "JPY"}, 12, "JP¥12"},
		{"de-AT", NumberOptions{Style: "currency", Currency: "EUR"}, 1234.5, "€\u00a01.234,50"},
		{"en", NumberOptions{Style: "currency", Currency: "USD", CurrencySign: "accounting"}, -5, "($5.00)"},
		{"en", NumberOptions{Style: "currency", Currency: "USD", CurrencyDisplay: "name"}, 2, "2.00 US dollars"},
		{"en-SI", NumberOptions{Style: "currency", Currency: "UAH", CurrencyDisplay: "name"}, 1234.5, "UAH 1,234.5 Ukrainian hryvnias0"},
		{"en", NumberOptions{Style: "unit", Unit: "kilometer-per-hour"}, 50, "50 km/h"},
		{"en", NumberOptions{Style: "unit", Unit: "byte", UnitDisplay: "long"}, 1, "1 byte"},
		{"fr", NumberOptions{Notation: "compact", CompactDisplay: "long"}, 1000, "mille"},
		{"fr", NumberOptions{Notation: "compact", CompactDisplay: "long"}, -1000, "-1 millier"},
		{"en", NumberOptions{Notation: "compact"}, 999999, "1M"},
		{"en", NumberOptions{Notation: "scientific"}, 123456, "1.235E5"},
		{"en", NumberOptions{MaximumSignificantDigits: 2.0}, 1234, "1,200"},
		{"en", NumberOptions{SignDisplay: "exceptZero"}, 0, "0"},
		{"en", NumberOptions{RoundingMode: "ceil", MaximumFractionDigits: 0.0}, 1.1, "2"},
	}
	for _, tt := range tests {
		if got := NewNumberFormat([]string{tt.locale}, tt.opts).Format(tt.value); got != tt.want {
			t.Errorf("NumberFormat(%s, %+v).Format(%v) = %q, want %q", tt.locale, tt.opts, tt.value, got, tt.want)
		}
	}
	if got := NewNumberFormat([]string{"en"}, NumberOptions{}).Format(math.Copysign(0, -1)); got != "-0" {
		t.Errorf("negative zero = %q, want %q", got, "-0")
	}
}

func TestPluralRules(t *testing.T) {
	tests := []struct {
		locale string
		opts   NumberOptions
		value  float64
		want   string
	}{
		{"en", NumberOptions{}, 1, "one"},
		{"en", NumberOptions{MinimumFractionDigits: 1.0}, 1, "other"},
		{"en", NumberOptions{Type: "ordinal"}, 22, "two"},
		{"ar", NumberOptions{}, 103, "few"},
		{"ru", NumberOptions{}, 21, "one"},
		{"cy", NumberOptions{Type: "ordinal"}, 8, "zero"},
		{"fr", NumberOptions{}, 1000000, "many"},
	}
	for _, tt := range tests {
		if got := NewPluralRules([]string{tt.locale}, tt.opts).Select(tt.value); got != tt.want {
			t.Errorf("PluralRules(%s, %+v).Select(%v) = %q, want %q", tt.locale, tt.opts, tt.value, got, tt.want)
		}
	}
}

func TestDateTimeFormat(t *testing.T) {
	const instant = 1720000000000 // 2024-07-03T09:46:40Z
	utc := func(o DateTimeOptions) DateTimeOptions {
		o.TimeZone = "UTC"
		return o
	}
	tests := []struct {
		locale string
		opts   DateTimeOptions
		want   string
	}{
		{"en-US", utc(DateTimeOptions{}), "7/3/2024"},
		{"de", utc(DateTimeOptions{DateStyle: "long"}), "3. Juli 2024"},
		{"en", utc(DateTimeOptions{DateStyle: "full", TimeStyle: "short"}), "Wednesday, July 3, 2024 at 9:46 AM"},
		{"en", utc(DateTimeOptions{Hour: "numeric", Hour12: false}), "09"},
		{"en", utc(DateTimeOptions{Hour: "numeric", DayPeriod: "short"}), "9 in the morning"},
		{"en", DateTimeOptions{TimeZone: "America/New_York", TimeZoneName: "long"}, "7/3/2024, Eastern Daylight Time"},
		{"en", DateTimeOptions{TimeZone: "Asia/Kolkata", TimeZoneName: "shortOffset"}, "7/3/2024, GMT+5:30"},
		{"en-u-ca-buddhist", utc(DateTimeOptions{Year: "numeric"}), "2567 BE"},
		{"fa", utc(DateTimeOptions{}), "۱۴۰۳/۴/۱۳"},
		{"en", utc(DateTimeOptions{Second: "numeric", FractionalSecondDigits: 2.0}), "40.00"},
	}
	for _, tt := range tests {
		if got := NewDateTimeFormat([]string{tt.locale}, tt.opts).Format(instant); got != tt.want {
			t.Errorf("DateTimeFormat(%s, %+v).Format = %q, want %q", tt.locale, tt.opts, got, tt.want)
		}
	}
}

func TestDateTimeNumberingAndOffsets(t *testing.T) {
	tests := []struct {
		locale string
		opts   DateTimeOptions
		value  float64
		want   string
	}{
		{"en-u-nu-arab", DateTimeOptions{TimeZone: "UTC", Second: "numeric", FractionalSecondDigits: 1.0}, 1000, "١٫٠"},
		{"ar", DateTimeOptions{TimeZone: "UTC", NumberingSystem: "mymr", Second: "numeric", FractionalSecondDigits: 1.0}, 1000, "၁.၀"},
		{"en", DateTimeOptions{TimeZone: "+00:00", TimeZoneName: "long", Hour: "numeric", Hour12: false}, 0, "00 Greenwich Mean Time"},
		{"ksh-DE-u-ca-buddhist", DateTimeOptions{TimeZone: "UTC", Year: "2-digit", Month: "2-digit", Minute: "numeric", Second: "numeric", FractionalSecondDigits: 3.0}, -2050448524945596, "−05-03 17:34,404"},
		{"az-u-nu-arab", DateTimeOptions{TimeZone: "+05:30", TimeZoneName: "short", Hour: "numeric", Hour12: false}, 0, "standart\u00a0onluq\u00a0kəsr٠٥ GMT+٥:٣٠"},
	}
	for _, tt := range tests {
		if got := NewDateTimeFormat([]string{tt.locale}, tt.opts).Format(tt.value); got != tt.want {
			t.Errorf("%s %+v: got %q, want %q", tt.locale, tt.opts, got, tt.want)
		}
	}
}

func TestInvalidTimeValue(t *testing.T) {
	defer func() {
		e, ok := recover().(*fluent.Error)
		if !ok || e.Name != "RangeError" || e.Message != "Invalid time value" {
			t.Errorf("recovered %v, want RangeError: Invalid time value", e)
		}
	}()
	NewDateTimeFormat([]string{"en"}, DateTimeOptions{}).Format(math.NaN())
}

func TestLocale(t *testing.T) {
	tests := []struct{ tag, language, script, region string }{
		{"en", "en", "Latn", "US"},
		{"zh-TW", "zh", "Hant", "TW"},
		{"sr-RU", "sr", "Cyrl", "RU"},
		{"und-Cyrl", "ru", "Cyrl", "RU"},
		{"iw", "he", "Hebr", "IL"},
		{"cmn", "zh", "Hans", "CN"},
	}
	for _, tt := range tests {
		m := NewLocale(tt.tag).Maximize()
		if m.Language() != tt.language || m.Script() != tt.script || m.Region() != tt.region {
			t.Errorf("Locale(%s).Maximize() = %s %s %s, want %s %s %s", tt.tag, m.Language(), m.Script(), m.Region(), tt.language, tt.script, tt.region)
		}
	}
}
