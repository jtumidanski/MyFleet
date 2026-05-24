package server

import (
	"net/http"
	"strconv"
)

type Page struct {
	Number int
	Size   int
}

type PageMeta struct {
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
	Number     int `json:"number"`
	Size       int `json:"size"`
}

func ParsePage(r *http.Request) Page {
	num := atoiDefault(r.URL.Query().Get("page[number]"), 1)
	if num < 1 {
		num = 1
	}
	size := atoiDefault(r.URL.Query().Get("page[size]"), 25)
	if size < 1 {
		size = 25
	}
	if size > 100 {
		size = 100
	}
	return Page{Number: num, Size: size}
}

func (p Page) Offset() int { return (p.Number - 1) * p.Size }

func (p Page) Meta(total int) PageMeta {
	pages := (total + p.Size - 1) / p.Size
	return PageMeta{Total: total, TotalPages: pages, Number: p.Number, Size: p.Size}
}

func atoiDefault(s string, d int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return d
}
