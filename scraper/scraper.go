package scraper

import (
	"fmt"
	"github.com/Jeffail/gabs/v2"
	"github.com/gocolly/colly/v2"
	"strings"
)

func parseJson(e *colly.Response) *gabs.Container {
	jsonParsed, err := gabs.ParseJSON(e.Body)
	if err != nil {
		panic(err)
	}
	return jsonParsed
}

type Film struct {
	Slug        string
	Url         string
	Id          string
	ServicesUrl string
	Name        string
}

type PriceEntry struct {
	Name     string
	Format   string
	Price    string
	PriceType     string
	FilmName string
	FilmId   string
	Url      string
}

func getServicesData(jsonParsed *gabs.Container, film Film) []PriceEntry {
	rent := jsonParsed.Path("best.rent")
	streaming := jsonParsed.Path("best.stream")
	prices := make([]PriceEntry, 0)
	prices = append(prices, getPrices(film, "streaming", streaming)...)
	prices = append(prices, getPrices(film, "rent", rent)...)
	return prices
}

func getPrices(film Film, priceType string, prices *gabs.Container) []PriceEntry {
	priceData := make([]PriceEntry, 0)
	for _, child := range prices.Children() {
		priceData = append(priceData, PriceEntry{
			Name:     child.Path("name").String(),
			Format:   child.Path("format").String(),
			Price:    child.Path("price").String(),
			PriceType:     priceType,
			FilmName: film.Name,
			FilmId:   film.Id,
			Url:      film.ServicesUrl,
		})
	}
	return priceData
}

func getFilm(element *colly.HTMLElement, filmChan chan Film) {
	film := Film{
		Slug:        element.Attr("data-target-link"),
		Url:         fmt.Sprintf("https://letterboxd.com/film%s", element.Attr("data-target-link")),
		Id:          element.Attr("data-film-id"),
		ServicesUrl: fmt.Sprintf("https://letterboxd.com/s/film-availability?filmId=%s&locale=USA", element.Attr("data-film-id")),
		Name:        element.Attr("data-film-name"),
	}
	filmChan <- film
}

func scrapeFilm(c *colly.Collector, url string) {
	c.OnHTML("section", func(e *colly.HTMLElement) {
		fmt.Println(e)
	})
	err := c.Visit(url)
	if err != nil {
		panic(err)
	}
}

func scrapeServices(c *colly.Collector, filmDataChan chan []PriceEntry, film Film) {
	c.OnResponse(func(e *colly.Response) {
		jsonParsed := parseJson(e)
		prices := getServicesData(jsonParsed, film)
		filmDataChan <- prices
	})
	err := c.Visit(film.ServicesUrl)
	if err != nil {
		panic(err)
	}
}

func ScrapeDirector(name string) {
	directorNameFmt := strings.Join(strings.Split(strings.ToLower(name), " "), "-")
	url := fmt.Sprintf("https://www.letterboxd.com/director/%s", directorNameFmt)
	fmt.Print(url)
	c := colly.NewCollector()
	filmChan := make(chan Film)
	filmDataChan := make(chan []PriceEntry)

	c.OnHTML("li.poster-container > div", func(e *colly.HTMLElement) {
		go getFilm(e, filmChan)
		// scrapeFilm(c, film.Url)
		go scrapeServices(c, filmDataChan, <-filmChan)
		data := <-filmDataChan
		for _, i2 := range data {
			fmt.Println(i2)
		}
	})
	err := c.Visit(url)
	if err != nil {
		fmt.Println(err)
	}

	c.Wait()
}
