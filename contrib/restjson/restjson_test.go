package restjson

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/i18n"

	"github.com/siyouyun-open/siyouyun_sdk/v2/appconst"
	"github.com/siyouyun-open/siyouyun_sdk/v2/apperr"
	"github.com/siyouyun-open/siyouyun_sdk/v2/paging"
)

// TestEnvelopeJSONShape pins the wire shape of the envelope.
//
// These strings are the contract the frontend unwraps against: field names, field order, and
// data being null rather than absent. If any one of them changes, the webui of the four
// applications breaks immediately, hence golden strings rather than "compare after
// unmarshalling".
func TestEnvelopeJSONShape(t *testing.T) {
	items := []string{"a", "b"}
	cases := []struct {
		name string
		got  any
		want string
	}{
		{
			name: "success",
			got:  SuccessResJson(),
			want: `{"code":"A00200","data":null,"msg":"ok"}`,
		},
		{
			name: "success-with-data",
			got:  SuccessResJsonWithData(&items),
			want: `{"code":"A00200","data":["a","b"],"msg":"ok"}`,
		},
		{
			name: "success-with-paging",
			got: SuccessResJsonWithPagingData(&PagingData{
				Pagination: paging.New(2, 10),
				Total:      42,
				Data:       []string{"a"},
			}),
			want: `{"code":"A00200","data":{"page":2,"pageSize":10,"total":42,"data":["a"]},"msg":"ok"}`,
		},
		{
			name: "param-error",
			got:  ErrorResJson(appconst.ParamError, "bad param"),
			want: `{"code":"A00400","data":null,"msg":"bad param"}`,
		},
		{
			name: "error-with-msg",
			got:  ErrorResJsonWithMsg("oops"),
			want: `{"code":"A00500","data":null,"msg":"oops"}`,
		},
		{
			name: "explicit-code",
			got:  ResJson(appconst.NotFound, &items, "gone"),
			want: `{"code":"A00404","data":["a","b"],"msg":"gone"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("incompatible envelope\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestErrorResJsonWithError covers both the i18n and the non-i18n rendering path.
//
// The message with a placeholder is a real defect of the old implementation (passing []any as
// a single argument to ctx.Tr, rendering as %!v(MISSING)); this asserts that the arguments are
// expanded correctly.
func TestErrorResJsonWithError(t *testing.T) {
	app := iris.New()
	if err := app.I18n.LoadKV(i18n.LangMap{
		"en-US": {"error.too_big": "file %s is too big"},
	}, "en-US"); err != nil {
		t.Fatalf("load i18n: %v", err)
	}
	app.Get("/t", func(ctx iris.Context) {
		switch ctx.URLParam("kind") {
		case "i18n":
			ctx.JSON(ErrorResJsonWithError(ctx, apperr.NewI18nError("error.too_big", "a.jpg")))
		case "nil":
			ctx.JSON(ErrorResJsonWithError(ctx, nil))
		default:
			ctx.JSON(ErrorResJsonWithError(ctx, errors.New("boom")))
		}
	})
	// Calling ServeHTTP directly requires Build first (the normal deployment path triggers it
	// via Listen).
	if err := app.Build(); err != nil {
		t.Fatalf("build iris app: %v", err)
	}

	cases := []struct {
		kind string
		want string
	}{
		{"plain", `{"code":"A00500","data":null,"msg":"boom"}`},
		{"i18n", `{"code":"A00500","data":null,"msg":"file a.jpg is too big"}`},
		{"nil", `{"code":"A00500","data":null,"msg":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/t?kind="+tc.kind, nil))
			// iris's ctx.JSON uses json.Encoder, which appends a trailing newline.
			if body := strings.TrimRight(rec.Body.String(), "\n"); body != tc.want {
				t.Fatalf("incompatible response\n got: %s\nwant: %s", body, tc.want)
			}
		})
	}
}

func TestNewPagination(t *testing.T) {
	app := iris.New()
	var got *paging.Pagination
	app.Get("/p", func(ctx iris.Context) {
		got = NewPagination(ctx)
		ctx.StatusCode(http.StatusNoContent)
	})
	if err := app.Build(); err != nil {
		t.Fatalf("build iris app: %v", err)
	}

	cases := []struct {
		query            string
		wantPage, wantSz int
	}{
		{"", 1, 10},
		{"?page=3&pageSize=25", 3, 25},
		{"?page=0&pageSize=0", 1, 10},
		{"?page=-5", 1, 10},
	}
	for _, tc := range cases {
		got = nil
		app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/p"+tc.query, nil))
		if got == nil {
			t.Fatalf("%s: handler was not executed", tc.query)
		}
		if got.Page != tc.wantPage || got.PageSize != tc.wantSz {
			t.Fatalf("%s: page=%d pageSize=%d, want %d/%d", tc.query, got.Page, got.PageSize, tc.wantPage, tc.wantSz)
		}
	}
}

func TestPaginationOffsetLimit(t *testing.T) {
	p := paging.New(3, 20)
	if p.Offset() != 40 || p.Limit() != 20 {
		t.Fatalf("offset/limit = %d/%d, want 40/20", p.Offset(), p.Limit())
	}
	var nilP *paging.Pagination
	if nilP.Offset() != 0 || nilP.Limit() != 10 {
		t.Fatalf("nil pagination = %d/%d, want 0/10", nilP.Offset(), nilP.Limit())
	}
}
