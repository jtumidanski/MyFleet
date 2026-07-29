package database

// Provider is a lazy, re-runnable data fetch (design §6: "Lazy data access").
type Provider[T any] func() (T, error)

func Query[T any](fetch func() (T, error)) Provider[T]          { return fetch }
func SliceQuery[T any](fetch func() ([]T, error)) Provider[[]T] { return fetch }
