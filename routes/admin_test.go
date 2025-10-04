package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/middleware/jwt"
)

// buildTestApp creates a minimal Iris app with the admin routes and JWT verifier
func buildTestApp() *iris.Application {
	os.Setenv("ACCESS_TOKEN_SECRET", "testsecret")
	app := iris.New()

	accessTokenVerifier := jwt.NewVerifier(jwt.HS256, []byte(os.Getenv("ACCESS_TOKEN_SECRET")))
	accessTokenVerifierMiddleware := accessTokenVerifier.Verify(func() interface{} { return new(mockAccessToken) })

	admin := app.Party("/api/admin", accessTokenVerifierMiddleware, mockAdminOnlyMiddleware)
	{
		admin.Get("/users", AdminListUsers)
	}
	return app
}

type mockAccessToken struct {
	ID   uint
	Role string
}

// mockAdminOnlyMiddleware uses mockAccessToken
func mockAdminOnlyMiddleware(ctx iris.Context) {
	claims := jwt.Get(ctx).(*mockAccessToken)
	if claims.Role != "admin" && claims.Role != "super_admin" {
		ctx.StatusCode(iris.StatusForbidden)
		return
	}
	ctx.Next()
}

// signTestToken returns a signed JWT with the given role
func signTestToken(role string) string {
	signer := jwt.NewSigner(jwt.HS256, os.Getenv("ACCESS_TOKEN_SECRET"), 0)
	token, _ := signer.Sign(mockAccessToken{ID: 1, Role: role})
	return string(token)
}

func TestAdminUsersRBAC(t *testing.T) {
	app := buildTestApp()

	// No token -> 401 handled by verifier, but Iris returns 401 only when route requires token; we simulate missing token
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	resp := httptest.NewRecorder()
	app.ServeHTTP(resp, req)
	if resp.Code == http.StatusOK {
		t.Fatalf("expected non-200 without token, got %d", resp.Code)
	}

	// User role -> 403
	req2 := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req2.Header.Set("Authorization", "Bearer "+signTestToken("user"))
	resp2 := httptest.NewRecorder()
	app.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for user role, got %d", resp2.Code)
	}

	// Admin role -> 200 (empty list OK)
	req3 := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req3.Header.Set("Authorization", "Bearer "+signTestToken("admin"))
	resp3 := httptest.NewRecorder()
	app.ServeHTTP(resp3, req3)
	if resp3.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin role, got %d", resp3.Code)
	}
}
