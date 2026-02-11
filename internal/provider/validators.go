package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Valid Notion colors for select/multi-select options.
var validColors = []string{
	"default", "gray", "brown", "orange", "yellow",
	"green", "blue", "purple", "pink", "red",
}

// Valid number formats for number properties.
var validNumberFormats = []string{
	"number", "number_with_commas", "percent", "dollar",
	"canadian_dollar", "euro", "pound", "yen", "ruble",
	"rupee", "won", "yuan", "real", "lira", "rupiah",
	"franc", "hong_kong_dollar", "new_zealand_dollar",
	"krona", "norwegian_krone", "mexican_peso",
	"rand", "new_taiwan_dollar", "danish_krone",
	"zloty", "baht", "forint", "koruna", "shekel",
	"chilean_peso", "philippine_peso", "dirham",
	"colombian_peso", "riyal", "ringgit", "leu",
	"argentine_peso", "uruguayan_peso", "singapore_dollar",
}

// Valid rollup functions.
var validRollupFunctions = []string{
	"count_all", "count_values", "count_unique_values",
	"count_empty", "count_not_empty",
	"percent_empty", "percent_not_empty",
	"sum", "average", "median",
	"min", "max", "range",
}

// colorValidator validates that a string is a valid Notion color.
type colorValidator struct{}

func (v colorValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %s", strings.Join(validColors, ", "))
}

func (v colorValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v colorValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, c := range validColors {
		if val == c {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Color",
		fmt.Sprintf("Expected one of: %s, got: %s", strings.Join(validColors, ", "), val),
	)
}

// ColorValidator returns a validator that checks for valid Notion colors.
func ColorValidator() validator.String {
	return colorValidator{}
}

// numberFormatValidator validates that a string is a valid Notion number format.
type numberFormatValidator struct{}

func (v numberFormatValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %s", strings.Join(validNumberFormats, ", "))
}

func (v numberFormatValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v numberFormatValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, f := range validNumberFormats {
		if val == f {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Number Format",
		fmt.Sprintf("Expected one of: %s, got: %s", strings.Join(validNumberFormats, ", "), val),
	)
}

// NumberFormatValidator returns a validator that checks for valid Notion number formats.
func NumberFormatValidator() validator.String {
	return numberFormatValidator{}
}

// rollupFunctionValidator validates that a string is a valid Notion rollup function.
type rollupFunctionValidator struct{}

func (v rollupFunctionValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of: %s", strings.Join(validRollupFunctions, ", "))
}

func (v rollupFunctionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v rollupFunctionValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	for _, f := range validRollupFunctions {
		if val == f {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Rollup Function",
		fmt.Sprintf("Expected one of: %s, got: %s", strings.Join(validRollupFunctions, ", "), val),
	)
}

// RollupFunctionValidator returns a validator that checks for valid Notion rollup functions.
func RollupFunctionValidator() validator.String {
	return rollupFunctionValidator{}
}
