package main

import (
	"github.com/gorilla/mux"
	"html/template"
	"letterboxd_scraper/scraper"
	"log"
	"net/http"
	"os"
)

type Response struct {
	Films []scraper.FilmEntry
	Price string
	ShowForm bool
}

func main() {
	router := mux.NewRouter()
	tmpl := template.Must(template.ParseFiles("form.html"))
	router.HandleFunc("/director", func(w http.ResponseWriter, r *http.Request) {
		streamingServices := []string{"Netflix", "HBO Max", "Amazon Prime Video", "TCM", "Criterion Channel"}
		if r.Method != http.MethodPost {
			tmpl.Execute(w,
			struct {
				StreamingServices []string
				ShowForm bool
			}{
				StreamingServices: streamingServices,
				ShowForm: true,
			})
			return
		}
		r.ParseForm()
		chosenStreamingServices := r.Form["streamingServices"]
		directorName := r.FormValue("name")
		films, price := scraper.ScrapeDirector(directorName, chosenStreamingServices)
		tmpl.Execute(w, Response{Films: films, Price: price, ShowForm: false})
	})

	port := os.Getenv("PORT") // Heroku provides the port to bind to
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, router))
}
