// Package apperr provides app-side internationalized errors: an error carries only an i18n key and
// arguments, and the response layer (restjson) renders it into text for the current request's
// language.
//
// The semantics are identical to the old SDK's `pkg/sdkerr`, which makes mechanical replacement easy.
package apperr

// I18nError is an internationalized error awaiting rendering.
type I18nError struct {
	Key  string
	Args []any
}

// Error returns the key itself. It is only used for logs and errors.Is/As checks;
// when facing the user it should be rendered by the response layer with ctx.Tr(key, args...).
func (e I18nError) Error() string {
	return e.Key
}

// NewI18nError constructs an internationalized error.
func NewI18nError(key string, args ...any) *I18nError {
	return &I18nError{Key: key, Args: args}
}
