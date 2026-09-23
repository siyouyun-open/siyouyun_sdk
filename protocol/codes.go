package protocol

// Response codes.
//
// Kept consistent with the A00xxx convention shared with the Core/OS services: the SDK
// relies only on these two values to decide "success" or "failure"; all other details are
// in the Error text.
const (
	// CodeSuccess indicates a successful call.
	CodeSuccess = "A00200"
	// CodeError indicates a failed call.
	CodeError = "A00400"
)
