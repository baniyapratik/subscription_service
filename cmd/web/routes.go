package main

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"net/http"
)

func (app *Config) routes() http.Handler {
	//create router
	mux := chi.NewRouter()

	// setup middleware
	mux.Use(middleware.Recoverer)

	// define application routes
	mux.Get("/", app.HomePage)

	return mux
}
