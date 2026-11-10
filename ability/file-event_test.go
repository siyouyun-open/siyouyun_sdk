package ability

import "testing"

func TestBuildExprSpec(t *testing.T) {
	if got, want := buildExprSpec([]string{"add"}, []string{".PSD", "svg"}), `action == "add" && !(ext in [".psd", ".svg"])`; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got, want := buildExprSpec([]string{"add", "rename"}, nil), `action in ["add", "rename"]`; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
