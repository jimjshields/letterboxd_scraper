package main

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"letterboxd_scraper/cache"
	"letterboxd_scraper/scraper"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
)

type Response struct {
	DirectorName string
	Films        []scraper.FilmPrices
	ShowForm     bool
	PriceDetails scraper.PriceDetails
}

var ctx = context.Background()

func main() {
	router := mux.NewRouter()
	tmpl := template.Must(template.ParseFiles("form.html"))
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		streamingServices := []string{"Netflix", "HBO Max", "Amazon Prime Video", "TCM", "Criterion Channel", "Hulu", "Showtime", "Apple TV Plus", "Disney Plus", "Starz", "HBO Go", "FX Now"}
		directors := getCachedDirectors()
		sort.Strings(streamingServices)
		if r.Method != http.MethodPost {
			tmpl.Execute(w,
				struct {
					StreamingServices []string
					ShowForm          bool
					Directors         []string
				}{
					StreamingServices: streamingServices,
					ShowForm:          true,
					Directors:         directors,
				})
			return
		}
		r.ParseForm()
		chosenStreamingServices := r.Form["streamingServices"]
		directorName := getDirectorName(r)
		films, priceDetails := scraper.ScrapeDirector(ctx, directorName, chosenStreamingServices)
		tmpl.Execute(w, Response{DirectorName: strings.Title(directorName), Films: films, PriceDetails: priceDetails, ShowForm: false})
	})

	port := os.Getenv("PORT") // Heroku provides the port to bind to
	if port == "" {
		port = "8080"
	}
	staticDir := "/static/"

	// Create the route
	router.PathPrefix(staticDir).Handler(http.StripPrefix(staticDir, http.FileServer(http.Dir("."+staticDir))))
	log.Fatal(http.ListenAndServe(":"+port, router))
}

func getCachedDirectors() []string {
	client := cache.RedisClient()
	directors, err := client.SMembers(ctx, scraper.DirectorsKey).Result()
	if err != nil {
		fmt.Println(err)
	}
	client.Close()
	sort.Strings(directors)
	return directors
}

func getDirectorName(request *http.Request) string {
	fromInput := request.FormValue("directorName")
	fromSelect := request.FormValue("directorNameSelect")
	if fromInput != "" {
		return fromInput
	} else {
		return fromSelect
	}
}
