package main

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"letterboxd_scraper/scraper"
	"net/http"
)

type Response struct {
	Films []scraper.FilmEntry
	Price float64
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/director/{name}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		directorName := vars["name"]
		streamingServices := []string{"Netflix", "HBO Max", "Amazon Prime Video", "TCM", "Criterion Channel"}
		films, price := scraper.ScrapeDirector(directorName, streamingServices)
		json.NewEncoder(w).Encode(Response{Films: films, Price: price})
	})

	http.ListenAndServe(":8080", router)
}
