// Ported from fluent.js (Apache-2.0), fluent-bundle/src/builtins.ts.

package bundle

import "maps"

// unwrap is valueOf on a function's named argument.
func unwrap(value FluentValue) any {
	if wrapped, ok := value.(FluentType); ok {
		return wrapped.ValueOf()
	}
	return value
}

// firstArgument is args[0], which is undefined when there are no arguments.
func firstArgument(positional []FluentValue) FluentValue {
	if len(positional) == 0 {
		return Undefined
	}
	return positional[0]
}

// number is the NUMBER() builtin. Options other than the allowed ones are ignored.
func number(positional []FluentValue, named map[string]FluentValue) FluentValue {
	switch arg := firstArgument(positional).(type) {
	case *FluentNone:
		return NewFluentNone("NUMBER(" + arg.Value + ")")
	case *FluentNumber:
		return NewFluentNumber(arg.Value, withNumberOptions(arg.Opts, named))
	case *FluentDateTime:
		return NewFluentNumber(arg.ToNumber(), withNumberOptions(NumberOptions{}, named))
	}
	panic(typeError("Invalid argument to NUMBER"))
}

// withNumberOptions returns opts with the allowed named arguments merged over them.
func withNumberOptions(opts NumberOptions, named map[string]FluentValue) NumberOptions {
	for name, value := range named {
		if field := numberOption(&opts, name); field != nil {
			*field = unwrap(value)
		}
	}
	return opts
}

// dateTime is the DATETIME() builtin. Options other than the allowed ones are ignored.
func dateTime(positional []FluentValue, named map[string]FluentValue) FluentValue {
	switch arg := firstArgument(positional).(type) {
	case *FluentNone:
		return NewFluentNone("DATETIME(" + arg.Value + ")")
	case *FluentDateTime, *FluentNumber:
		var opts DateTimeOptions
		for name, value := range named {
			if field := dateTimeOption(&opts, name); field != nil {
				*field = unwrap(value)
			}
		}
		return NewFluentDateTime(arg, opts)
	}
	panic(typeError("Invalid argument to DATETIME"))
}

// numberOption returns the field of a NUMBER() option, or nil if it is not allowed.
func numberOption(opts *NumberOptions, name string) *any {
	switch name {
	case "unitDisplay":
		return &opts.UnitDisplay
	case "currencyDisplay":
		return &opts.CurrencyDisplay
	case "useGrouping":
		return &opts.UseGrouping
	case "minimumIntegerDigits":
		return &opts.MinimumIntegerDigits
	case "minimumFractionDigits":
		return &opts.MinimumFractionDigits
	case "maximumFractionDigits":
		return &opts.MaximumFractionDigits
	case "minimumSignificantDigits":
		return &opts.MinimumSignificantDigits
	case "maximumSignificantDigits":
		return &opts.MaximumSignificantDigits
	}
	return nil
}

// dateTimeOption returns the field of a DATETIME() option, or nil if it is not allowed.
func dateTimeOption(opts *DateTimeOptions, name string) *any {
	switch name {
	case "dateStyle":
		return &opts.DateStyle
	case "timeStyle":
		return &opts.TimeStyle
	case "fractionalSecondDigits":
		return &opts.FractionalSecondDigits
	case "dayPeriod":
		return &opts.DayPeriod
	case "hour12":
		return &opts.Hour12
	case "weekday":
		return &opts.Weekday
	case "era":
		return &opts.Era
	case "year":
		return &opts.Year
	case "month":
		return &opts.Month
	case "day":
		return &opts.Day
	case "hour":
		return &opts.Hour
	case "minute":
		return &opts.Minute
	case "second":
		return &opts.Second
	case "timeZoneName":
		return &opts.TimeZoneName
	}
	return nil
}

// builtins returns NUMBER and DATETIME under the given custom functions.
func builtins(custom map[string]FluentFunction) map[string]FluentFunction {
	functions := map[string]FluentFunction{"NUMBER": number, "DATETIME": dateTime}
	maps.Copy(functions, custom)
	return functions
}
