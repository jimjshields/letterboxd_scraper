package main

import (
	"letterboxd_scraper/scraper"
	"os"
)

func main() {
	directorName := os.Args[1]
	scraper.ScrapeDirector(directorName)
}
