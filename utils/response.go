package utils

import (
	"github.com/kataras/iris/v12"
)

type PageMeta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}

func JSONPage(ctx iris.Context, data interface{}, page, perPage int, total int64) {
	ctx.JSON(iris.Map{
		"data":  data,
		"meta":  PageMeta{Page: page, PerPage: perPage, Total: total},
		"links": iris.Map{},
	})
}

func JSONError(ctx iris.Context, status int, code, message string) {
	ctx.StatusCode(status)
	ctx.JSON(iris.Map{"error": code, "message": message})
}
