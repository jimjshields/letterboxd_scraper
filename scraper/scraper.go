package scraper

import (
	"fmt"
	"github.com/Jeffail/gabs/v2"
	"github.com/gocolly/colly/v2"
	"regexp"
	"strconv"
	"strings"
)

func ScrapeDirector(name string) []PriceEntry {
	url := getDirectorUrl(name)
	c := colly.NewCollector()
	var data []PriceEntry
	var filmChan = make(chan Film)
	var servicesData = make([]PriceEntry, 0)

	c.OnHTML("li.poster-container > div", func(e *colly.HTMLElement) {
		go getFilm(e, filmChan)
		// scrapeFilm(c, film.Url)
		servicesData = append(servicesData, scrapeServices(<-filmChan)...)
	})
	err := c.Visit(url)
	if err != nil {
		fmt.Println(err)
	}
	c.Wait()
	for _, priceEntry := range servicesData {
		fmt.Println(priceEntry.FilmName, priceEntry.Name, priceEntry.Price)
	}
	return data
}

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
	Name      string
	Format    string
	Price     float64
	PriceType string
	FilmName  string
	FilmId    string
	Url       string
}

func getServicesData(jsonParsed *gabs.Container, film Film) []PriceEntry {
	rent := jsonParsed.Path("best.rent")
	streaming := jsonParsed.Path("best.stream")
	prices := make([]PriceEntry, 0)
	for _, price := range getPrices(film, "streaming", streaming) {
		prices = append(prices, price)
	}
	for _, price := range getPrices(film, "rent", rent) {
		prices = append(prices, price)
	}
	return prices
}

func getPrices(film Film, priceType string, prices *gabs.Container) []PriceEntry {
	priceData := make([]PriceEntry, 0)
	for _, child := range prices.Children() {
		price := getPrice(child)
		priceData = append(priceData, PriceEntry{
			Name:      child.Path("name").String(),
			Format:    child.Path("format").String(),
			Price:     price,
			PriceType: priceType,
			FilmName:  film.Name,
			FilmId:    film.Id,
			Url:       film.ServicesUrl,
		})
	}
	return priceData
}

var priceRegex = regexp.MustCompile(`\d+\.\d+`)

func getPrice(child *gabs.Container) float64 {
	priceString := child.Path("price").String()
	var price float64
	var err error
	if !strings.Contains(priceString, "null") {
		price, err = strconv.ParseFloat(string(priceRegex.Find([]byte(priceString))), 64)
		if err != nil {
			panic(err)
		}
	} else {
		price = 0.0
	}
	return price
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

func scrapeServices(film Film) []PriceEntry {
	c := colly.NewCollector()
	prices := make([]PriceEntry, 0)
	c.OnResponse(func(e *colly.Response) {
		jsonParsed := parseJson(e)
		prices = getServicesData(jsonParsed, film)
	})
	err := c.Visit(film.ServicesUrl)
	if err != nil {
		panic(err)
	}
	return prices
}

func getDirectorUrl(name string) string {
	directorNameFmt := strings.Join(strings.Split(strings.ToLower(name), " "), "-")
	url := fmt.Sprintf("https://www.letterboxd.com/director/%s", directorNameFmt)
	return url
}
