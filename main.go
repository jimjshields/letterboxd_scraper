package main

import (
	"fmt"
	"letterboxd_scraper/scraper"
	"os"
)

func main() {
	directorName := os.Args[1]
	data := scraper.ScrapeDirector(directorName)
	for _, datum := range data {
		fmt.Printf(datum.FilmName, datum.Name, datum.Price)
	}
}
