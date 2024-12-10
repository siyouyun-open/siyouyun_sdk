package sdkerr

type I18nError struct {
	Key  string
	Args []any
}

func (e I18nError) Error() string {
	return e.Key
}

func NewI18nError(key string, args ...any) *I18nError {
	return &I18nError{
		Key:  key,
		Args: args,
	}
}
