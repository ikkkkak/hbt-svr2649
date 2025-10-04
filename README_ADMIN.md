Admin API

Setup
- ENV: ACCESS_TOKEN_SECRET, REFRESH_TOKEN_SECRET, DB_CONNECTION_STRING
- Run server: go run main.go

Auth & RBAC
- All /api/admin/* endpoints require a valid access token with role = admin or super_admin.
- Role change (PATCH /api/admin/users/{id}/role) requires super_admin.

Pagination & Errors
- Responses shape: { data, meta: { page, per_page, total }, links } for list endpoints.
- Errors: { error, message } with appropriate HTTP status codes.

Audit Logs
- Mutating endpoints record an audit row with before/after snapshots and admin IP.

Export Jobs
- POST /api/admin/export returns { id, status } and processes asynchronously (demo in-memory store).
- GET /api/admin/export/{id} returns job status.

OpenAPI
- See openapi_admin.yaml.

Postman
- Import openapi_admin.yaml in Postman to generate a collection.

Demo Flows (curl)
```
# List pending properties
curl -H "Authorization: Bearer $TOKEN" "https://hbt-svr2649.onrender.com/api/admin/properties?status=pending&page=1&per_page=25"

# Approve property
curl -X PATCH -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" \
  -d '{"status":"approved","note":"Verified host documents."}' \
  "https://hbt-svr2649.onrender.com/api/admin/properties/123/status"

# Verify user
curl -X POST -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" \
  -d '{"status":"verified","notes":"Passport verified"}' \
  "https://hbt-svr2649.onrender.com/api/admin/users/42/verify"

# Remove a video comment
curl -X DELETE -H "Authorization: Bearer $TOKEN" \
  "https://hbt-svr2649.onrender.com/api/admin/videos/55/comments/777"
```

