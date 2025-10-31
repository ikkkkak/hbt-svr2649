package routes

import (
    "github.com/kataras/iris/v12"
)

// Temporary alias to avoid 404s from legacy client calls to /api/conversations
// Returns an empty list for now; wire to real data if needed
func ListConversationsAlias(ctx iris.Context) {
    ctx.JSON([]interface{}{})
}


